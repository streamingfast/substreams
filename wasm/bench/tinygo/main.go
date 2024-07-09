package main

import (
	"github.com/streamingfast/tinygo-test/pb"
)

//go:generate substreams go-gen

func map_test(block *pb.Block) (*pb.Block, error) {
	logf("where does this go?")
	block.Number = 2
	return block, nil
}
