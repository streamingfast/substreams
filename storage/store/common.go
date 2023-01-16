package store

import (
	"bytes"
	"context"
	"fmt"
	"github.com/streamingfast/derr"
	"github.com/streamingfast/dstore"
	"io"
)

func saveStore(ctx context.Context, store dstore.Store, filename string, content []byte) error {
	return derr.RetryContext(ctx, 5, func(ctx context.Context) error {
		return store.WriteObject(ctx, filename, bytes.NewReader(content))
	})
}

func loadStore(ctx context.Context, store dstore.Store, filename string) (out []byte, err error) {
	err = derr.RetryContext(ctx, 5, func(ctx context.Context) error {
		r, err := store.OpenObject(ctx, filename)
		if err != nil {
			return fmt.Errorf("openning file: %w", err)
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
