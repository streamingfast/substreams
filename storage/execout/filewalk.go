package execout

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/streamingfast/substreams/block"
)

// FileWalker allows you to jump from file to file, from segment to segment
type FileWalker struct {
	config    *Config
	segmenter *block.Segmenter
	segment   int

	buffer           map[int]*File
	bufferLock       sync.Mutex
	previousFileSize atomic.Uint64
}

func (c *Config) NewFileWalker(segmenter *block.Segmenter) *FileWalker {
	return &FileWalker{
		config:    c,
		segmenter: segmenter,
		segment:   segmenter.FirstIndex(),
		buffer:    make(map[int]*File),
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
	// we can preload two next files if they are small enough.
	// More than 2 shows no performance improvement and gobbles up memory.
	if fw.segment != fw.segmenter.FirstIndex() && fw.previousFileSize.Load() < 104_857_600 {
		fw.preload(ctx, fw.segment+2)
	}

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
			// purposefully ignoring preload errors
			fw.previousFileSize.Store(f.loadedSize)
		}
	}()
	fw.buffer[seg] = f
}

// Move to the next
func (fw *FileWalker) Next() {
	fw.segment++
}

func (fw *FileWalker) IsDone() bool {
	return fw.segment > fw.segmenter.LastIndex()
}

func (fw *FileWalker) Progress() (first, current, last int) {
	return fw.segmenter.FirstIndex(), fw.segment, fw.segmenter.LastIndex()
}
