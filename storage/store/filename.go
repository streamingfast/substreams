package store

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/streamingfast/substreams/block"
)

var stateFileRegex = regexp.MustCompile(`([\d]+)-([\d]+)(?:\.([^\.]+))?\.(kv|partial)`)

type FileInfos []*FileInfo

func (f FileInfos) Ranges() (out block.Ranges) {
	if len(f) == 0 {
		return nil
	}

	out = make(block.Ranges, len(f))
	for i, file := range f {
		out[i] = file.Range
	}
	return
}

func (f FileInfos) String() string {
	ranges := make([]string, len(f))
	for i, file := range f {
		ranges[i] = file.Range.String()
	}

	return strings.Join(ranges, ",")
}

type FileInfo struct {
	ModuleName  string
	Filename    string
	Range       *block.Range
	Partial     bool
	WithTraceID bool
}

func NewCompleteFileInfo(moduleName string, moduleInitialBlock uint64, exclusiveEndBlock uint64) *FileInfo {
	bRange := block.NewRange(moduleInitialBlock, exclusiveEndBlock)

	return &FileInfo{
		ModuleName: moduleName,
		Filename:   FullStateFileName(bRange),
		Range:      block.NewRange(moduleInitialBlock, exclusiveEndBlock),
		Partial:    false,
	}
}

func NewPartialFileInfo(moduleName string, start uint64, exclusiveEndBlock uint64) *FileInfo {
	bRange := block.NewRange(start, exclusiveEndBlock)

	return &FileInfo{
		ModuleName: moduleName,
		Filename:   PartialFileName(bRange),
		Range:      bRange,
		Partial:    true,
	}
}

func parseFileName(moduleName, filename string) (*FileInfo, bool) {
	res := stateFileRegex.FindAllStringSubmatch(filename, 1)
	if len(res) != 1 {
		return nil, false
	}

	return &FileInfo{
		ModuleName:  moduleName,
		Filename:    filename,
		Range:       block.NewRange(uint64(mustAtoi(res[0][2])), uint64(mustAtoi(res[0][1]))),
		Partial:     res[0][4] == "partial",
		WithTraceID: res[0][3] != "",
	}, true
}

func PartialFileName(r *block.Range) string {
	return fmt.Sprintf("%010d-%010d.partial", r.ExclusiveEndBlock, r.StartBlock)
}

func FullStateFileName(r *block.Range) string {
	return fmt.Sprintf("%010d-%010d.kv", r.ExclusiveEndBlock, r.StartBlock)
}

func mustAtoi(s string) int {
	i, err := strconv.Atoi(s)
	if err != nil {
		panic(err)
	}
	return i
}
