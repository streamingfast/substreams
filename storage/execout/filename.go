package execout

import (
	"fmt"
	"regexp"

	"github.com/streamingfast/substreams/block"
)

var cacheFilenameRegex *regexp.Regexp
var indexFilenameRegex *regexp.Regexp

func init() {
	cacheFilenameRegex = regexp.MustCompile(`([\d]+)-([\d]+)\.output`)
	indexFilenameRegex = regexp.MustCompile(`([\d]+)-([\d]+)\.index`)
}

type FileInfos = []*FileInfo

type FileInfo struct {
	Filename   string
	BlockRange *block.Range
}

func parseExecoutFileName(filename string) (*FileInfo, error) {
	blockRange, err := fileNameToRange(filename, cacheFilenameRegex)
	if err != nil {
		return nil, fmt.Errorf("parsing filename %q: %w", filename, err)
	}
	return &FileInfo{
		Filename:   filename,
		BlockRange: blockRange,
	}, nil
}

func parseIndexFileName(filename string) (*FileInfo, error) {
	blockRange, err := fileNameToRange(filename, indexFilenameRegex)
	if err != nil {
		return nil, fmt.Errorf("parsing filename %q: %w", filename, err)
	}
	return &FileInfo{
		Filename:   filename,
		BlockRange: blockRange,
	}, nil
}

func fileNameToRange(filename string, regex *regexp.Regexp) (*block.Range, error) {
	res := regex.FindAllStringSubmatch(filename, 1)
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
