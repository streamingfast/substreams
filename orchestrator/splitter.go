package orchestrator

import (
	"github.com/streamingfast/substreams/block"
)

type Splitter struct {
	chunkSize uint64
}

func NewSplitter(chunkSize uint64) *Splitter {
	return &Splitter{
		chunkSize: chunkSize,
	}
}

func (s *Splitter) Split(moduleInitialBlock uint64, lastSavedBlock uint64, blockRange *block.Range) []*block.Range {
	if moduleInitialBlock > blockRange.StartBlock {
		blockRange.StartBlock = moduleInitialBlock
	}

	if lastSavedBlock > blockRange.StartBlock {
		blockRange.StartBlock = lastSavedBlock
	}

	return blockRange.Split(s.chunkSize)
}
