package store

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"math/big"

	"github.com/shopspring/decimal"
	"github.com/streamingfast/dmetering"

	"github.com/streamingfast/derr"
	"github.com/streamingfast/dstore"
)

func saveStore(ctx context.Context, store dstore.Store, filename string, content []byte) (err error) {
	if cloned, ok := store.(dstore.Clonable); ok {
		store, err = cloned.Clone(ctx)
		if err != nil {
			return fmt.Errorf("cloning store: %w", err)
		}
		store.SetMeter(dmetering.GetBytesMeter(ctx))
	}

	return derr.RetryContext(ctx, 10, func(ctx context.Context) error { // more than the usual 5 retries because if we fail, we have to reprocess the whole segment
		return store.WriteObject(ctx, filename, bytes.NewReader(content))
	})
}

func loadStore(ctx context.Context, store dstore.Store, filename string) (out []byte, err error) {
	if cloned, ok := store.(dstore.Clonable); ok {
		store, err = cloned.Clone(ctx)
		if err != nil {
			return nil, fmt.Errorf("cloning store: %w", err)
		}
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
