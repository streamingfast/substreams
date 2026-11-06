package store

import (
	"context"
	"fmt"
	"io"

	"github.com/streamingfast/dstore"
	"github.com/streamingfast/substreams/storage/store/marshaller"
)

type fileWriter struct {
	store    dstore.Store
	filename string
	reader   io.ReadCloser
}

func (f *fileWriter) Write(ctx context.Context) error {
	defer f.reader.Close()
	return saveStoreStream(ctx, f.store, f.filename, f.reader)
}

// retryableSave is Save's writer for a caller that may retry. fileWriter's reader is a
// one-shot stream consumed by its first Write — a retry would need a fresh Save, which
// reopens the snapshot against whatever the store looks like *then*, not at the endBlock
// the filename promises. Write here instead rewinds the same snapshot (see
// marshaller.KVSnapshotIter.Reset) and re-streams it from the start on every attempt, so a
// retry can never upload different (newer) content than the attempt that failed. The
// snapshot — and, on mmap's long-lived-snapshot mode, its open read transaction — stays
// alive across every attempt. The caller MUST Close() once done, retry loop finished or
// not. See FullKV.SaveRetryable.
type retryableSave struct {
	store          dstore.Store
	filename       string
	marshaller     marshaller.StreamMarshaller
	snap           marshaller.KVSnapshotIter
	deletePrefixes []string
}

func (w *retryableSave) Write(ctx context.Context) error {
	if err := w.snap.Reset(); err != nil {
		return fmt.Errorf("rewinding snapshot for retry: %w", err)
	}
	reader := w.marshaller.MarshalStreamSnapshot(w.snap, w.deletePrefixes)
	return saveStoreStream(ctx, w.store, w.filename, reader)
}

func (w *retryableSave) Close() error {
	return w.snap.Close()
}
