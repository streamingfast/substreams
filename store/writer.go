package store

import (
	"context"
	"github.com/streamingfast/dstore"
)

type fileWriter struct {
	store    dstore.Store
	filename string
	content  []byte
}

func (f *fileWriter) Write(ctx context.Context) error {
	return saveStore(ctx, f.store, f.filename, f.content)
}
