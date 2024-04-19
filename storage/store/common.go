package store

import (
	"bytes"
	"context"
	"fmt"
	"io"

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
