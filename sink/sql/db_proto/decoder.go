package db_proto

import (
	"fmt"
	"runtime"
	"sync"
	"time"

	sql "github.com/streamingfast/substreams/sink/sql/db_proto/sql"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"
)

// decoder turns buffered blocks into buffered inserts, on several goroutines.
//
// Unmarshalling into dynamicpb and walking the message descriptor is where the sink
// spends its time — measured at ~7.8µs of the ~9.3µs a wide entity costs, see
// sink/sql/db_proto/benchmarks. Blocks are independent, so that part parallelises
// cleanly. Everything that touches the database stays on the caller's goroutine: the
// workers only fill a BufferedInserter each, and the caller replays them in block
// order.
type decoder struct {
	rootMessageDescriptor protoreflect.MessageDescriptor
	workers               int

	// buffers are reused across flushes to keep the per-row []any allocations from
	// being handed to the GC every batch.
	buffers []*sql.BufferedInserter
}

// decoded is one block's worth of buffered inserts, plus what it cost to produce.
type decoded struct {
	blockNum       uint64
	blockHash      string
	blockTimestamp time.Time
	inserts        *sql.BufferedInserter
	// empty marks a block whose module produced no output. Such a block is skipped
	// entirely, including its _blocks_ row, as the serial path always did.
	empty bool

	unmarshalDuration time.Duration
	walkDuration      time.Duration
}

func newDecoder(rootMessageDescriptor protoreflect.MessageDescriptor, workers int) *decoder {
	if workers <= 0 {
		// One per core, less one for the goroutine draining the gRPC stream, capped at
		// 8: TestClientDecodeScaling measures 4.13x at eight workers and only 4.24x at
		// fifteen, the work being allocator-bound well before it runs out of cores.
		// Taking more would cost the rest of the machine for nothing.
		workers = min(8, max(1, runtime.NumCPU()-1))
	}

	return &decoder{
		rootMessageDescriptor: rootMessageDescriptor,
		workers:               workers,
	}
}

// decodeAll walks every held block and returns the results in the same order.
//
// The database is only read for its dialect and descriptors, both immutable once the
// sinker is running, so this is safe to run concurrently. Nothing here inserts.
func (d *decoder) decodeAll(holding []*Holder, db sql.Database) ([]*decoded, error) {
	results := make([]*decoded, len(holding))
	errs := make([]error, len(holding))

	d.ensureBuffers(len(holding))

	if d.workers == 1 || len(holding) == 1 {
		for i, holder := range holding {
			results[i], errs[i] = d.decodeOne(holder, db, d.buffers[i])
		}
		return collect(results, errs)
	}

	var (
		wg   sync.WaitGroup
		next = make(chan int)
	)

	wg.Add(d.workers)
	for range d.workers {
		go func() {
			defer wg.Done()
			for i := range next {
				results[i], errs[i] = d.decodeOne(holding[i], db, d.buffers[i])
			}
		}()
	}

	for i := range holding {
		next <- i
	}
	close(next)
	wg.Wait()

	return collect(results, errs)
}

func collect(results []*decoded, errs []error) ([]*decoded, error) {
	for i, err := range errs {
		if err != nil {
			return nil, fmt.Errorf("decoding block at index %d: %w", i, err)
		}
	}

	return results, nil
}

// ensureBuffers grows the reusable buffer pool to one per held block and resets them.
func (d *decoder) ensureBuffers(count int) {
	for len(d.buffers) < count {
		d.buffers = append(d.buffers, sql.NewBufferedInserter(64))
	}
	for i := range count {
		d.buffers[i].Reset()
	}
}

func (d *decoder) decodeOne(holder *Holder, db sql.Database, into *sql.BufferedInserter) (*decoded, error) {
	clock := holder.data.Clock
	out := &decoded{
		blockNum:       clock.Number,
		blockHash:      clock.Id,
		blockTimestamp: clock.Timestamp.AsTime(),
		inserts:        into,
	}

	payload := holder.output.GetMapOutput().GetValue()
	if len(payload) == 0 {
		out.empty = true
		return out, nil
	}

	unmarshalStartAt := time.Now()
	message := dynamicpb.NewMessage(d.rootMessageDescriptor)
	if err := proto.Unmarshal(payload, message); err != nil {
		return nil, fmt.Errorf("unmarshaling message: %w", err)
	}
	out.unmarshalDuration = time.Since(unmarshalStartAt)

	walkStartAt := time.Now()
	if _, err := db.WalkMessageDescriptorAndInsertInto(message, out.blockNum, out.blockTimestamp, nil, into); err != nil {
		return nil, fmt.Errorf("processing message %q: %w", string(message.Descriptor().FullName()), err)
	}
	out.walkDuration = time.Since(walkStartAt)

	return out, nil
}

// apply performs the decoded inserts against the database, in block order. It runs on
// the caller's goroutine, so the database sees exactly the sequence it would have seen
// from a serial walk.
func (d *decoder) apply(results []*decoded, db sql.Database) (time.Duration, error) {
	insertDuration := time.Duration(0)

	for _, result := range results {
		if result.empty {
			continue
		}

		startAt := time.Now()

		if err := db.InsertBlock(result.blockNum, result.blockHash, result.blockTimestamp); err != nil {
			return 0, fmt.Errorf("inserting block %d: %w", result.blockNum, err)
		}
		if err := result.inserts.Replay(db); err != nil {
			return 0, fmt.Errorf("applying block %d: %w", result.blockNum, err)
		}

		insertDuration += time.Since(startAt)
	}

	return insertDuration, nil
}
