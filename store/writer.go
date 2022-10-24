package store

import (
	"context"
	"github.com/streamingfast/dstore"
)

type FileWriter struct {
	store    dstore.Store
	filename string
	content  []byte
}

func (f *FileWriter) Write(ctx context.Context) error {
	return saveStore(ctx, f.store, f.filename, f.content)
}
