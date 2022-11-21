package execout

import (
	"fmt"
	"regexp"

	"github.com/streamingfast/substreams/block"
)

var cacheFilenameRegex *regexp.Regexp

func init() {
	cacheFilenameRegex = regexp.MustCompile(`([\d]+)-([\d]+)\.output`)
}

type FileInfos = []*FileInfo

type FileInfo struct {
	Filename   string
	BlockRange *block.Range
}

func parseFileName(filename string) (*FileInfo, error) {
	blockRange, err := fileNameToRange(filename)
	if err != nil {
		return nil, fmt.Errorf("parsing filename %q: %w", filename, err)
	}
	return &FileInfo{
		Filename:   filename,
		BlockRange: blockRange,
	}, nil
}
func fileNameToRange(filename string) (*block.Range, error) {
	res := cacheFilenameRegex.FindAllStringSubmatch(filename, 1)
	if len(res) != 1 {
		return nil, fmt.Errorf("invalid output cache filename, %q", filename)
	}

	start := uint64(mustAtoi(res[0][1]))
	end := uint64(mustAtoi(res[0][2]))

	return &block.Range{
		StartBlock:        start,
		ExclusiveEndBlock: end,
	}, nil
}
