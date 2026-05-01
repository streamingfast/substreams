package writer

import (
	"context"
	"fmt"
	"io"

	"github.com/streamingfast/dstore"
)

type dataFile struct {
	reader         io.Reader
	outputFilename string
}

func (d *dataFile) Upload(ctx context.Context, store dstore.Store) (string, error) {
	if err := store.WriteObject(ctx, d.outputFilename, d.reader); err != nil {
		return "", fmt.Errorf("write object: %w", err)
	}
	return store.ObjectPath(d.outputFilename), nil
}

type localFile struct {
	localFilePath  string
	outputFilename string
}

func (l *localFile) Upload(ctx context.Context, store dstore.Store) (string, error) {
	if err := store.PushLocalFile(ctx, l.localFilePath, l.outputFilename); err != nil {
		return "", fmt.Errorf("pushing  object: %w", err)
	}
	return store.ObjectPath(l.outputFilename), nil
}
