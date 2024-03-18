package store

import "github.com/streamingfast/substreams/block"

type FileInfoParam interface {
	apply(file *FileInfo)
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

		file.Filename = PartialFileName(blockRange)
		files[i] = file
	}

	return files
}
