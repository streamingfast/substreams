package writer

import (
	"fmt"

	"github.com/streamingfast/bstream"
	"go.uber.org/zap"
)

type FileType string

const (
	FileTypeJSONL FileType = "jsonl"
	FileTypeCSV   FileType = "csv"
)

type baseWriter struct {
	fileType FileType
	zlogger  *zap.Logger
}

func newBaseWriter(fileType FileType, zlogger *zap.Logger) baseWriter {
	return baseWriter{
		fileType: fileType,
		zlogger:  zlogger,
	}

}

func (b baseWriter) filename(blockRange *bstream.Range) string {
	return fmt.Sprintf("%010d-%010d", blockRange.StartBlock(), *blockRange.EndBlock()-1)
}

func (b baseWriter) Type() FileType {
	return b.fileType
}
