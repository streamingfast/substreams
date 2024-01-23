package execout

import "github.com/streamingfast/substreams/block"

// FileWalker allows you to jump from file to file, from segment to segment
type FileWalker struct {
	config    *Config
	segmenter *block.Segmenter
	segment   int
}

func (c *Config) NewFileWalker(segmenter *block.Segmenter) *FileWalker {
	return &FileWalker{
		config:    c,
		segmenter: segmenter,
		segment:   segmenter.FirstIndex(),
	}
}

// File returns the current segment's file.
// If the current segment is out of ranges, returns nil.
func (fw *FileWalker) File() *File {
	rng := fw.segmenter.Range(fw.segment)
	if rng == nil {
		return nil
	}
	return fw.config.NewFile(rng)
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
