package index

import (
	"context"
	"fmt"

	"github.com/streamingfast/substreams/block"

	"go.uber.org/zap"

	"github.com/RoaringBitmap/roaring/roaring64"

	"github.com/streamingfast/dstore"
	"github.com/streamingfast/substreams/manifest"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

type Writer struct {
	indexFile *File
}

func NewWriter(indexFile *File) *Writer {
	return &Writer{
		indexFile: indexFile,
	}
}

func (w *Writer) Write(indexes map[string]*roaring64.Bitmap) {
	w.indexFile.Set(indexes)
}

func (w *Writer) Close(ctx context.Context) error {
	err := w.indexFile.Save(ctx)
	if err != nil {
		return fmt.Errorf("saving index file %s: %w", w.indexFile.moduleName, err)
	}

	return nil
}

func GenerateBlockIndexWriters(baseStore dstore.Store, indexModules []*pbsubstreams.Module, ModuleHashes *manifest.ModuleHashes, logger *zap.Logger, blockRange *block.Range) (map[string]*Writer, error) {
	indexFiles := make(map[string]*File)
	for _, module := range indexModules {
		currentFile, err := NewFile(baseStore, ModuleHashes.Get(module.Name), module.Name, logger, blockRange)
		if err != nil {
			return nil, fmt.Errorf("creating new index file for %q: %w", module.Name, err)
		}
		indexFiles[module.Name] = currentFile
	}

	indexWriters := make(map[string]*Writer)
	for moduleName, file := range indexFiles {
		indexWriters[moduleName] = NewWriter(file)
	}
	return indexWriters, nil
}
