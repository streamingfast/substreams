package db_proto

import (
	"fmt"
	"runtime"
	"sync"
	"time"

	"buf.build/go/hyperpb"
	sql "github.com/streamingfast/substreams/sink/sql/db_proto/sql"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// decoder turns buffered blocks into buffered inserts, on several goroutines.
//
// Unmarshalling the payload and walking the message descriptor is where the sink spends
// its time — see sink/sql/db_proto/benchmarks. Blocks are independent, so that part
// parallelises cleanly. Everything that touches the database stays on the caller's
// goroutine: the workers only fill a BufferedInserter each, and the caller replays them
// in block order.
//
// The parser is hyperpb rather than protobuf-go's dynamicpb. Both are driven purely by
// the module's descriptor, which is all the sink has at runtime, and both are read
// through protoreflect by the walk — but hyperpb compiles the descriptor into a parser
// once and then parses into an arena, which BenchmarkUnmarshal measures at 15-17x
// dynamicpb with one allocation per block instead of thousands. That last part is the
// one that matters here: TestClientDecodeScaling flattens at eight workers because the
// decode path is allocator-bound, so removing the allocations lifts the ceiling the
// worker pool runs into, not just the single-threaded time.
type decoder struct {
	// messageType is the parser compiled from the module's output descriptor. Compiling
	// is expensive and the result is immutable and safe to share, so it happens once per
	// sinker.
	messageType *hyperpb.MessageType
	workers     int

	// buffers are reused across flushes to keep the per-row []any allocations from
	// being handed to the GC every batch.
	buffers []*sql.BufferedInserter
	// arenas hold the parsed messages, one per buffer slot.
	//
	// hyperpb parses into an arena and hands out strings and byte slices that point into
	// it, so the arena has to outlive every value the walk extracted. Those values sit in
	// buffers[i] until apply replays them, which is why an arena is only freed at the top
	// of the *next* decodeAll — by then the previous round has been applied and flushed,
	// and neither the buffer nor any inserter still refers to it.
	//
	// Freeing does not return the memory to the OS, it returns it to hyperpb for reuse,
	// so a slot's arena settles at roughly the high-water mark of the blocks that landed
	// in it. That is the same shape as buffers above.
	arenas []*arena
}

// arena owns one hyperpb.Shared and the bookkeeping the decoder needs around it.
//
// Two rules come from hyperpb rather than from us, and both are panics rather than
// errors, so they are enforced here instead of being left to callers:
//
//   - Free panics if nothing was ever allocated. A block whose module produced no output
//     never parses, so a slot can reach its first free untouched. used covers that.
//   - A Shared holds exactly one parse: parsing again before Free panics with "attempted
//     to parse message using in-use Context". The decoder satisfies this structurally by
//     giving every block in a batch its own slot, and freeing a whole round at once.
type arena struct {
	shared *hyperpb.Shared
	used   bool
}

func newArena() *arena {
	return &arena{shared: new(hyperpb.Shared)}
}

// parse decodes one payload. The message, and every string and byte slice reachable from
// it, is backed by the arena and stays valid until free.
func (a *arena) parse(messageType *hyperpb.MessageType, payload []byte) (*hyperpb.Message, error) {
	a.used = true

	message := a.shared.NewMessage(messageType)
	if err := message.Unmarshal(payload); err != nil {
		return nil, err
	}

	return message, nil
}

// free reclaims the arena, invalidating everything the last parse produced.
func (a *arena) free() {
	if !a.used {
		return
	}

	a.shared.Free()
	a.used = false
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
	// insertDuration is what this one block cost on the serial path — the block row plus
	// replaying its buffered rows into the database or the spool. It is measured per
	// block rather than per batch so the average reads in the same unit as the two above
	// it, which is what the stats panel claims of all three.
	insertDuration time.Duration
}

// ResolveDecodeWorkers turns a requested worker count into the one that will be used.
//
// One per core, less one for the goroutine draining the gRPC stream, capped at 8:
// TestClientDecodeScaling measures 4.52x at eight workers and 5.06x at fifteen, so the
// seven extra cores together buy 11%. Taking them would cost the rest of the machine for
// almost nothing.
func ResolveDecodeWorkers(workers int) int {
	if workers > 0 {
		return workers
	}

	return min(8, max(1, runtime.NumCPU()-1))
}

func newDecoder(rootMessageDescriptor protoreflect.MessageDescriptor, workers int) *decoder {
	workers = ResolveDecodeWorkers(workers)

	return &decoder{
		messageType: hyperpb.CompileMessageDescriptor(rootMessageDescriptor),
		workers:     workers,
	}
}

// decodeAll walks every held block and returns the results in the same order.
//
// The database is only read for its dialect and descriptors, both immutable once the
// sinker is running, so this is safe to run concurrently. Nothing here inserts.
func (d *decoder) decodeAll(holding []*Holder, db sql.Database) ([]*decoded, error) {
	results := make([]*decoded, len(holding))
	errs := make([]error, len(holding))

	d.ensureSlots(len(holding))

	if d.workers == 1 || len(holding) == 1 {
		for i, holder := range holding {
			results[i], errs[i] = d.decodeOne(holder, db, d.buffers[i], d.arenas[i])
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
				results[i], errs[i] = d.decodeOne(holding[i], db, d.buffers[i], d.arenas[i])
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

// ensureSlots grows the reusable per-block pools to one entry per held block, and clears
// the entries this round is about to use.
//
// This is the only place an arena is freed, and the reason is ordering: the rows the last
// round produced alias arena memory and are consumed by apply, which the sinker calls
// before it can come back here. Freeing on the way in therefore reclaims memory that is
// provably dead, where freeing on the way out of decodeAll would pull it out from under
// the rows still waiting to be replayed.
func (d *decoder) ensureSlots(count int) {
	// Every arena is freed, not only the ones this round will use: a shorter batch after
	// a longer one would otherwise leave the tail slots pinning the previous round's
	// memory until a batch that large came along again.
	for _, arena := range d.arenas {
		arena.free()
	}

	for len(d.buffers) < count {
		d.buffers = append(d.buffers, sql.NewBufferedInserter(64))
		d.arenas = append(d.arenas, newArena())
	}
	for i := range count {
		d.buffers[i].Reset()
	}
}

func (d *decoder) decodeOne(holder *Holder, db sql.Database, into *sql.BufferedInserter, arena *arena) (*decoded, error) {
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
	message, err := arena.parse(d.messageType, payload)
	if err != nil {
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
// apply replays every decoded block in order, timing each one so the stats can report a
// per-block cost rather than a per-batch total.
func (d *decoder) apply(results []*decoded, db sql.Database) error {
	for _, result := range results {
		if result.empty {
			continue
		}

		startAt := time.Now()

		if err := db.InsertBlock(result.blockNum, result.blockHash, result.blockTimestamp); err != nil {
			return fmt.Errorf("inserting block %d: %w", result.blockNum, err)
		}
		if err := result.inserts.Replay(db); err != nil {
			return fmt.Errorf("applying block %d: %w", result.blockNum, err)
		}

		result.insertDuration = time.Since(startAt)
	}

	return nil
}
