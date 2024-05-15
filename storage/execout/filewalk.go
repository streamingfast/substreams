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

	IsLocal          bool
	buffer           map[int]*File
	bufferLock       sync.Mutex
	previousFileSize atomic.Uint64

	currentlyPreloadingSegments map[int]chan bool
	currentlyPreloadingLock     sync.RWMutex
}

func (c *Config) NewFileWalker(segmenter *block.Segmenter) *FileWalker {
	return &FileWalker{
		config:                      c,
		IsLocal:                     c.objStore.BaseURL().Scheme == "file",
		segmenter:                   segmenter,
		segment:                     segmenter.FirstIndex(),
		buffer:                      make(map[int]*File),
		currentlyPreloadingSegments: make(map[int]chan bool),
	}
}

// File returns the current segment's file.
// If the current segment is out of ranges, returns nil.
func (fw *FileWalker) File() *File {
	fw.currentlyPreloadingLock.RLock()
	wait, ok := fw.currentlyPreloadingSegments[fw.segment]
	fw.currentlyPreloadingLock.RUnlock()

	if ok {
		// wait for the file to be loaded. this channel is closed when the file is loaded
		<-wait

		// remove the segment from the map
		fw.currentlyPreloadingLock.Lock()
		delete(fw.currentlyPreloadingSegments, fw.segment)
		fw.currentlyPreloadingLock.Unlock()
	}

	rng := fw.segmenter.Range(fw.segment)
	if rng == nil {
		return nil
	}

	fw.bufferLock.Lock()
	defer fw.bufferLock.Unlock()

	if file, found := fw.buffer[fw.segment]; found {
		delete(fw.buffer, fw.segment)
		file.preloaded = true
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

	fw.currentlyPreloadingLock.Lock()
	fw.currentlyPreloadingSegments[seg] = make(chan bool)
	fw.currentlyPreloadingLock.Unlock()

	f := fw.config.NewFile(rng)
	fw.buffer[seg] = f

	go func(file *File) {
		defer func() {
			fw.currentlyPreloadingLock.Lock()
			if _, found := fw.currentlyPreloadingSegments[seg]; found {
				close(fw.currentlyPreloadingSegments[seg])
			}
			fw.currentlyPreloadingLock.Unlock()
		}()

		if err := file.Load(ctx); err == nil {
			// purposefully ignoring preload errors
			fw.previousFileSize.Store(f.loadedSize)
		}
	}(f)
}

// Move to the next
func (fw *FileWalker) Next() {
	fw.segment++

	fw.bufferLock.Lock()
	keys := make([]int, 0, len(fw.buffer))
	for k := range fw.buffer {
		keys = append(keys, k)
	}
	fw.bufferLock.Unlock()

	// delete older segments from the buffer
	for _, k := range keys {
		if k < fw.segment {
			fw.bufferLock.Lock()
			fw.currentlyPreloadingLock.Lock()

			delete(fw.buffer, k)
			delete(fw.currentlyPreloadingSegments, k)

			fw.currentlyPreloadingLock.Unlock()
			fw.bufferLock.Unlock()
		}
	}
}

func (fw *FileWalker) IsDone() bool {
	return fw.segment > fw.segmenter.LastIndex()
}

func (fw *FileWalker) Progress() (first, current, last int) {
	return fw.segmenter.FirstIndex(), fw.segment, fw.segmenter.LastIndex()
}
