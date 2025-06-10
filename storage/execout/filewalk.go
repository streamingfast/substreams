package execout

import (
	"context"

	"go.uber.org/zap"

	"github.com/streamingfast/substreams/block"
)

// FileWalker allows you to jump from file to file, from segment to segment
type FileWalker struct {
	config    *Config
	segmenter *block.Segmenter
	segment   int

	IsLocal bool
	logger  *zap.Logger
}

func NewFileWalker(c *Config, segmenter *block.Segmenter, logger *zap.Logger) *FileWalker {
	return &FileWalker{
		config:    c,
		IsLocal:   c.objStore.BaseURL().Scheme == "file",
		segmenter: segmenter,
		segment:   segmenter.FirstIndex(),
		logger:    logger,
	}
}

// If the current segment is out of ranges, returns nil.
func (fw *FileWalker) FileReader(ctx context.Context) (FileReader, error) {
	rng := fw.segmenter.Range(fw.segment)
	if rng == nil {
		return nil, nil
	}
	return fw.config.NewFileReader(ctx, rng)
}

// If the current segment is out of ranges, returns nil.
func (fw *FileWalker) FileWriter(ctx context.Context) FileWriter {
	rng := fw.segmenter.Range(fw.segment)
	if rng == nil {
		return nil
	}
	return fw.config.NewFileWriter(ctx, rng)
}

func (fw *FileWalker) Next() {
	fw.segment++
}

func (fw *FileWalker) IsDone() bool {
	return fw.segment > fw.segmenter.LastIndex()
}

func (fw *FileWalker) Progress() (first, current, last int) {
	return fw.segmenter.FirstIndex(), fw.segment, fw.segmenter.LastIndex()
}
