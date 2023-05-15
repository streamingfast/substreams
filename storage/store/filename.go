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
	Filename string
	Range    *block.Range
	TraceID  string
	Partial  bool
}

func NewCompleteFileInfo(moduleInitialBlock uint64, exlusiveEnd uint64) *FileInfo {
	bRange := block.NewRange(moduleInitialBlock, exlusiveEnd)

	return &FileInfo{
		Filename: FullStateFileName(bRange),
		Range:    block.NewRange(moduleInitialBlock, exlusiveEnd),
		Partial:  false,
	}
}

func NewPartialFileInfo(start uint64, exlusiveEnd uint64, traceID string) *FileInfo {
	bRange := block.NewRange(start, exlusiveEnd)

	return &FileInfo{
		Filename: PartialFileName(bRange, traceID),
		Range:    bRange,
		TraceID:  traceID,
		Partial:  true,
	}
}

func parseFileName(filename string) (*FileInfo, bool) {
	res := stateFileRegex.FindAllStringSubmatch(filename, 1)
	if len(res) != 1 {
		return nil, false
	}

	return &FileInfo{
		Filename: filename,
		Range:    block.NewRange(uint64(mustAtoi(res[0][2])), uint64(mustAtoi(res[0][1]))),
		TraceID:  res[0][3],
		Partial:  res[0][4] == "partial",
	}, true
}

// CompleteFiles returns a list of FileInfo for the given ranges, infallibly, panics
// on errors, ideal for tests.
func CompleteFiles(in string, params ...FileInfoParam) FileInfos {
	return fileFromRanges("complete", in, params...)
}

// PartialFiles returns a list of FileInfo for the given ranges, infallibly, panics
// on errors, ideal for tests.
func PartialFiles(in string, params ...FileInfoParam) FileInfos {
	return fileFromRanges("partial", in, params...)
}

// PartialFile returns a FileInfo for the given range, infallibly, panics
// on errors, ideal for tests.
func PartialFile(in string, params ...FileInfoParam) *FileInfo {
	return fileFromRange("partial", in, params...)
}

// CompleteFile returns a FileInfo for the given range, infallibly, panics
// on errors, ideal for tests.
func CompleteFile(in string, params ...FileInfoParam) *FileInfo {
	return fileFromRange("complete", in, params...)
}

func fileFromRange(kind string, in string, params ...FileInfoParam) *FileInfo {
	ranges := fileFromRanges(kind, in, params...)
	if len(ranges) == 0 {
		return nil
	}

	return ranges[0]
}

func fileFromRanges(kind string, in string, params ...FileInfoParam) FileInfos {
	ranges := block.ParseRanges(in)

	files := make([]*FileInfo, len(ranges))
	for i, blockRange := range ranges {
		file := &FileInfo{
			Range:   blockRange,
			Partial: kind == "partial",
		}

		for _, param := range params {
			param.apply(file)
		}

		file.Filename = PartialFileName(blockRange, file.TraceID)
		files[i] = file
	}

	return files
}

type FileInfoParam interface {
	apply(file *FileInfo)
}

type TraceIDParam string

func (t TraceIDParam) apply(file *FileInfo) {
	file.TraceID = string(t)
}

func PartialFileName(r *block.Range, traceID string) string {
	if traceID == "" {
		// Generate legacy partial filename
		return fmt.Sprintf("%010d-%010d.partial", r.ExclusiveEndBlock, r.StartBlock)
	}

	return fmt.Sprintf("%010d-%010d.%s.partial", r.ExclusiveEndBlock, r.StartBlock, traceID)
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
