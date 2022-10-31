package store

import (
	"fmt"
	"regexp"
	"strconv"

	"github.com/streamingfast/substreams/block"
)

var stateFileRegex *regexp.Regexp

func init() {
	stateFileRegex = regexp.MustCompile(`([\d]+)-([\d]+)\.(kv|partial)`)
}

type FileInfo struct {
	Filename   string
	StartBlock uint64
	EndBlock   uint64
	Partial    bool
}

func parseFileName(filename string) (*FileInfo, bool) {
	res := stateFileRegex.FindAllStringSubmatch(filename, 1)
	if len(res) != 1 {
		return nil, false
	}

	end := uint64(mustAtoi(res[0][1]))
	start := uint64(mustAtoi(res[0][2]))
	partial := res[0][3] == "partial"

	return &FileInfo{
		Filename:   filename,
		StartBlock: start,
		EndBlock:   end,
		Partial:    partial,
	}, true
}

func fullStateFilePrefix(blockNum uint64) string {
	return fmt.Sprintf("%010d", blockNum)
}

func partialFileName(r *block.Range) string {
	return fmt.Sprintf("%010d-%010d.partial", r.ExclusiveEndBlock, r.StartBlock)
}

func fullStateFileName(r *block.Range) string {
	return fmt.Sprintf("%010d-%010d.kv", r.ExclusiveEndBlock, r.StartBlock)
}

func mustAtoi(s string) int {
	i, err := strconv.Atoi(s)
	if err != nil {
		panic(err)
	}
	return i
}
