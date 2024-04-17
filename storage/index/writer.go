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

// startblock=500
// look for 0->1000

// GenrateBlockIndexWriters will only generate writers for modules that have no preexisting index file and that are aligned with the bundle size
func GenerateBlockIndexWriters(ctx context.Context, baseStore dstore.Store, indexModules []*pbsubstreams.Module, ModuleHashes *manifest.ModuleHashes, logger *zap.Logger, blockRange *block.Range, bundleSize uint64) (writers map[string]*Writer, existingIndices map[string]map[string]*roaring64.Bitmap, err error) {
	writers = make(map[string]*Writer)
	existingIndices = make(map[string]map[string]*roaring64.Bitmap)

	isAligned := blockRange.StartBlock%bundleSize == 0 && blockRange.ExclusiveEndBlock%bundleSize == 0
	if !isAligned { // we align it, but won't write it because it would be missing blocks...
		alignedStartBlock := blockRange.StartBlock - (blockRange.StartBlock % bundleSize)
		blockRange = &block.Range{
			StartBlock:        alignedStartBlock,
			ExclusiveEndBlock: alignedStartBlock + bundleSize,
		}
	}

	for _, module := range indexModules {
		currentFile, err := NewFile(baseStore, ModuleHashes.Get(module.Name), module.Name, logger, blockRange)
		if err != nil {
			return nil, nil, fmt.Errorf("creating new index file for %q: %w", module.Name, err)
		}
		if err := currentFile.Load(ctx); err == nil {
			existingIndices[module.Name] = currentFile.indexes
			continue
		}

		if !isAligned {
			continue
		}
		writers[module.Name] = NewWriter(currentFile)

	}

	return
}
