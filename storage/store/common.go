package store

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"iter"
	"math"
	"math/big"

	"github.com/streamingfast/derr"
	"github.com/streamingfast/dmetering"
	"github.com/streamingfast/dstore"
	"github.com/streamingfast/substreams/metering"
	"github.com/streamingfast/substreams/reqctx"
	"github.com/streamingfast/substreams/storage/store/marshaller"

	"github.com/shopspring/decimal"
)

func saveStore(ctx context.Context, store dstore.Store, filename string, content []byte) (err error) {
	if cloned, ok := store.(dstore.Clonable); ok {
		store, err = cloned.Clone(ctx, metering.WithBytesMeteringOptions(dmetering.GetBytesMeter(ctx), reqctx.Logger(ctx))...)
		if err != nil {
			return fmt.Errorf("cloning store: %w", err)
		}
		//todo: (deprecated)
		store.SetMeter(dmetering.GetBytesMeter(ctx))
	}

	return derr.RetryContext(ctx, 10, func(ctx context.Context) error { // more than the usual 5 retries because if we fail, we have to reprocess the whole segment
		return store.WriteObject(ctx, filename, bytes.NewReader(content))
	})
}

func saveStoreStream(ctx context.Context, store dstore.Store, filename string, reader io.Reader) (err error) {
	if cloned, ok := store.(dstore.Clonable); ok {
		store, err = cloned.Clone(ctx, metering.WithBytesMeteringOptions(dmetering.GetBytesMeter(ctx), reqctx.Logger(ctx))...)
		if err != nil {
			return fmt.Errorf("cloning store: %w", err)
		}
		//todo: (deprecated)
		store.SetMeter(dmetering.GetBytesMeter(ctx))
	}

	// We cannot do a retry here, because the reader cannot be reset.
	return store.WriteObject(ctx, filename, reader)
}

func loadStore(ctx context.Context, store dstore.Store, filename string) (out []byte, err error) {
	if cloned, ok := store.(dstore.Clonable); ok {
		store, err = cloned.Clone(ctx, metering.WithBytesMeteringOptions(dmetering.GetBytesMeter(ctx), reqctx.Logger(ctx))...)
		if err != nil {
			return nil, fmt.Errorf("cloning store: %w", err)
		}
		//todo: (deprecated)
		store.SetMeter(dmetering.GetBytesMeter(ctx))
	}

	err = derr.RetryContext(ctx, 5, func(ctx context.Context) error {
		r, err := store.OpenObject(ctx, filename)
		if err != nil {
			return fmt.Errorf("opening file: %w", err)
		}

		defer r.Close()
		data, err := io.ReadAll(r)
		if err != nil {
			return fmt.Errorf("reading data: %w", err)
		}

		out = data
		return nil
	})
	return out, err
}

func loadStoreStream(ctx context.Context, store dstore.Store, filename string) (reader io.ReadCloser, err error) {
	if cloned, ok := store.(dstore.Clonable); ok {
		store, err = cloned.Clone(ctx, metering.WithBytesMeteringOptions(dmetering.GetBytesMeter(ctx), reqctx.Logger(ctx))...)
		if err != nil {
			return nil, fmt.Errorf("cloning store: %w", err)
		}
		//todo: (deprecated)
		store.SetMeter(dmetering.GetBytesMeter(ctx))
	}

	err = derr.RetryContext(ctx, 5, func(ctx context.Context) error {
		r, err := store.OpenObject(ctx, filename)
		if err != nil {
			return fmt.Errorf("opening file for streaming: %w", err)
		}
		reader = r
		return nil
	})

	return
}

// unmarshalIterInto streams entries from an UnmarshalIter call directly into the kvImpl.
func unmarshalIterInto(ctx context.Context, impl KVImpl, um marshaller.StreamMarshaller, reader io.Reader, onTrailer func(deletePrefixes []string)) (uint64, error) {
	var totalSizeBytes uint64

	trailer, err := impl.Load(withCancelCheck(ctx, um.UnmarshalIter(reader, 10*1024*1024)))

	if err != nil {
		return 0, fmt.Errorf("kv load: %w", err)
	}

	if trailer != nil {
		totalSizeBytes = trailer.TotalSizeBytes
		if onTrailer != nil {
			onTrailer(trailer.DeletePrefixes)
		}
	}

	return totalSizeBytes, nil
}

// withCancelCheck wraps a store-entry iterator so a canceled/expired context
// aborts the load promptly. Without it, Load reads the entire (multi-GB) store
// file into the heap even for a request that is already dead, then returns
// ctx.Err() only at the very end. The check is throttled to avoid a ctx.Err()
// call on every single entry.
func withCancelCheck(ctx context.Context, it iter.Seq2[marshaller.StoreDataEntry, error]) iter.Seq2[marshaller.StoreDataEntry, error] {
	return func(yield func(marshaller.StoreDataEntry, error) bool) {
		const checkEvery = 1024
		n := 0
		for entry, err := range it {
			if n%checkEvery == 0 {
				if ctxErr := ctx.Err(); ctxErr != nil {
					yield(marshaller.StoreDataEntry{}, ctxErr)
					return
				}
			}
			n++
			if !yield(entry, err) {
				return
			}
		}
	}
}

// apparently this is faster than append() method
func cloneBytes(b []byte) []byte {
	out := make([]byte, len(b))
	copy(out, b)
	return out
}

func bigIntToBytes(i *big.Int) []byte {
	return []byte(i.String())
}

func float64ToBytes(f float64) []byte {
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], math.Float64bits(f))
	return buf[:]
}

func int64ToBytes(i int64) []byte {
	big := new(big.Int)
	big.SetInt64(i)
	return []byte(big.String())
}

func bigDecimalToBytes(d decimal.Decimal) []byte {
	val, err := d.MarshalBinary()
	if err != nil {
		panic(err)
	}
	return val
}
