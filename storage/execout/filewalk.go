package execout

import (
	"context"
	"sync"

	"go.uber.org/zap"

	"github.com/streamingfast/substreams/block"
)

// FileWalker allows you to jump from file to file, from segment to segment
type FileWalker struct {
	config    *Config
	segmenter *block.Segmenter
	segment   int

	IsLocal    bool
	buffer     map[int]*File
	bufferLock sync.Mutex

	logger *zap.Logger
}

func NewFileWalker(c *Config, segmenter *block.Segmenter, logger *zap.Logger) *FileWalker {
	return &FileWalker{
		config:    c,
		IsLocal:   c.objStore.BaseURL().Scheme == "file",
		segmenter: segmenter,
		segment:   segmenter.FirstIndex(),
		buffer:    make(map[int]*File),
		logger:    logger,
	}
}

// File returns the current segment's file.
// If the current segment is out of ranges, returns nil.
func (fw *FileWalker) File() *File {
	rng := fw.segmenter.Range(fw.segment)
	if rng == nil {
		return nil
	}

	fw.bufferLock.Lock()
	defer fw.bufferLock.Unlock()
	if file, found := fw.buffer[fw.segment]; found {
		delete(fw.buffer, fw.segment)
		return file
	}

	return fw.config.NewFile(rng)
}

// PreloadNext loads the next file in the background so the consumer doesn't wait between each file.
// This affects maximum throughput
func (fw *FileWalker) PreloadNext(ctx context.Context) {
	fw.bufferLock.Lock()
	defer fw.bufferLock.Unlock()
	fw.preload(ctx, fw.segment+1)
}

func (fw *FileWalker) preload(ctx context.Context, seg int) {
	if _, found := fw.buffer[seg]; found {
		return
	}
	rng := fw.segmenter.Range(seg)
	if rng == nil {
		return
	}

	f := fw.config.NewFile(rng)
	go func() {
		if err := f.Load(ctx); err == nil {
		}
	}()
	fw.buffer[seg] = f
}

// Move to the next
func (fw *FileWalker) Next() {
	fw.segment++

	fw.bufferLock.Lock()
	defer fw.bufferLock.Unlock()

	// delete old buffer
	oldBuffersFound := 0
	for k := range fw.buffer {
		if k < fw.segment {
			oldBuffersFound++
			delete(fw.buffer, k)
		}
	}
	if oldBuffersFound > 0 {
		fw.logger.Warn("deleted old buffers", zap.String("module", fw.config.name), zap.Int("count", oldBuffersFound))
	}
}

func (fw *FileWalker) IsDone() bool {
	return fw.segment > fw.segmenter.LastIndex()
}

func (fw *FileWalker) Progress() (first, current, last int) {
	return fw.segmenter.FirstIndex(), fw.segment, fw.segmenter.LastIndex()
}
