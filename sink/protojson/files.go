package protojson

// This package serves as a stub that re-exports the necessary types and functions
// from github.com/streamingfast/substreams-sink-files/v2 to avoid direct imports
// in the command files.

import (
	"fmt"

	substreamsfile "github.com/streamingfast/substreams-sink-files/v2"
	"github.com/streamingfast/substreams-sink-files/v2/bundler"
	"github.com/streamingfast/substreams-sink-files/v2/bundler/writer"
	"github.com/streamingfast/substreams-sink-files/v2/encoder"
	"github.com/streamingfast/substreams-sink-files/v2/protox"
	"github.com/streamingfast/substreams-sink-files/v2/state"
	sink "github.com/streamingfast/substreams/sink"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// Re-export types and functions from substreams-sink-files/v2
var (
	NewFileSinker            = substreamsfile.NewFileSinker
	NewBundler               = bundler.New
	NewBufferedIO            = writer.NewBufferedIO
	NewProtoToJson           = encoder.NewProtoToJson
	NewFileStateStore        = state.NewFileStateStore
	FindMessageByNameInFiles = protox.FindMessageByNameInFiles
)

// Re-export types
type (
	FileSinker  = *substreamsfile.FileSinker
	Bundler     = *bundler.Bundler
	Writer      = writer.Writer
	Encoder     = encoder.Encoder
	EncoderFunc = encoder.EncoderFunc
	FileType    = writer.FileType
)

// Re-export constants
const (
	FileTypeJSONL = writer.FileTypeJSONL
)

// Shared helper functions

func OutputMessageDescriptor(sinker *sink.Sinker) (protoreflect.MessageDescriptor, error) {
	outputTypeName := protoreflect.FullName(sinker.OutputModuleTypeUnprefixed())
	value, err := FindMessageByNameInFiles(sinker.Package().ProtoFiles, outputTypeName)
	if err != nil {
		return nil, fmt.Errorf("find message by name in files: %w", err)
	}

	if value == nil {
		return nil, fmt.Errorf("output module %q descriptor not found in proto files", outputTypeName)
	}

	return value, nil
}

func OutputProtoreflectMessageDescriptor(sinker *sink.Sinker) (protoreflect.MessageDescriptor, error) {
	outputTypeName := protoreflect.FullName(sinker.OutputModuleTypeUnprefixed())
	value, err := FindMessageByNameInFiles(sinker.Package().ProtoFiles, outputTypeName)
	if err != nil {
		return nil, fmt.Errorf("find message by name in files: %w", err)
	}

	if value == nil {
		return nil, fmt.Errorf("output module %q descriptor not found in proto files", outputTypeName)
	}

	return value, nil
}
