package codegen

import (
	"fmt"

	"github.com/jhump/protoreflect/desc"

	"github.com/streamingfast/substreams/manifest"
)

func Init() *Generator {
	var protoDefinitions []*desc.FileDescriptor
	manifestPath := "./test_substreams/substreams.yaml"
	manifestReader := manifest.NewReader(manifestPath, manifest.SkipSourceCodeReader(), manifest.WithCollectProtoDefinitions(func(pd []*desc.FileDescriptor) {
		protoDefinitions = pd
	}))

	pkg, err := manifestReader.Read()
	if err != nil {
		panic(fmt.Errorf("reading manifest file %s :%w", manifestPath, err))
	}
	return NewGenerator(pkg, protoDefinitions, "")
}
