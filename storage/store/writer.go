package store

import (
	"context"
	"io"

	"github.com/streamingfast/dstore"
)

type fileWriter struct {
	store    dstore.Store
	filename string
	reader   io.ReadCloser

	// metadataKeyValues is written with the file, as key-value pairs.
	metadataKeyValues []string
}

func (f *fileWriter) Write(ctx context.Context) error {
	defer f.reader.Close()
	return saveStoreStream(ctx, f.store, f.filename, f.reader, f.metadataKeyValues...)
}
