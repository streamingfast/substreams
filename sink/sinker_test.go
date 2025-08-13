package sink

import (
	"testing"

	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/descriptorpb"
)

func TestRequestIncludesProtoFiles(t *testing.T) {
	// Create a mock package with proto files
	pkg := &pbsubstreams.Package{
		ProtoFiles: []*descriptorpb.FileDescriptorProto{
			{
				Name: stringPtr("test.proto"),
			},
		},
		Modules: &pbsubstreams.Modules{
			Modules: []*pbsubstreams.Module{
				{
					Name: "test_module",
					Kind: &pbsubstreams.Module_KindMap_{
						KindMap: &pbsubstreams.Module_KindMap{
							OutputType: "test.TestOutput",
						},
					},
				},
			},
		},
	}

	// Create a sinker config with the mock package
	config := &SinkerConfig{
		Pkg: pkg,
		OutputModule: &pbsubstreams.Module{
			Name: "test_module",
			Kind: &pbsubstreams.Module_KindMap_{
				KindMap: &pbsubstreams.Module_KindMap{
					OutputType: "test.TestOutput",
				},
			},
		},
	}

	// Create a sinker with the config
	sinker := &Sinker{
		SinkerConfig: config,
	}

	// Create a request
	sinker.request = &pbsubstreamsrpc.Request{
		StartBlockNum:  1,
		StopBlockNum:   100,
		Modules:        pkg.Modules,
		OutputModule:   "test_module",
		ProductionMode: false,
		ProtoFiles:     pkg.ProtoFiles,
	}

	// Verify that the request includes the proto files
	request := sinker.Request()
	require.NotNil(t, request)
	assert.Equal(t, pkg.ProtoFiles, request.ProtoFiles)
	assert.Len(t, request.ProtoFiles, 1)
	assert.Equal(t, "test.proto", *request.ProtoFiles[0].Name)
}

func stringPtr(s string) *string {
	return &s
}

