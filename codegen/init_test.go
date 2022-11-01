package codegen

import (
	"fmt"
	"testing"

	"github.com/jhump/protoreflect/desc"
	"github.com/streamingfast/substreams/manifest"
	"github.com/stretchr/testify/require"
)

func InitTestGenerator(t *testing.T) *Generator {
	t.Helper()

	var protoDefinitions []*desc.FileDescriptor
	manifestPath := "./test_substreams/substreams.yaml"
	manifestReader := manifest.NewReader(manifestPath, manifest.SkipSourceCodeReader(), manifest.WithCollectProtoDefinitions(func(pd []*desc.FileDescriptor) {
		protoDefinitions = pd
	}))

	pkg, err := manifestReader.Read()
	if err != nil {
		panic(fmt.Errorf("reading manifest file %s :%w", manifestPath, err))
	}

	manif, err := manifest.LoadManifestFile(manifestPath)
	require.NoError(t, err)

	return NewGenerator(pkg, manif, protoDefinitions, "")
}
