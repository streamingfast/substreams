package sink

import (
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/protoc-gen-go/descriptor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
)

func TestRequestIncludesProtoFiles(t *testing.T) {
	// Create a mock package with proto files
	mockProtoFile := &descriptor.FileDescriptorProto{
		Name:    proto.String("test.proto"),
		Package: proto.String("test"),
	}

	pkg := &pbsubstreams.Package{
		ProtoFiles: []*descriptor.FileDescriptorProto{mockProtoFile},
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

	// Create a sinker with the mock package
	sinker := &Sinker{
		Pkg: pkg,
		SinkerConfig: &SinkerConfig{
			OutputModule: &pbsubstreams.Module{
				Name: "test_module",
			},
		},
	}

	// Create a request
	req := sinker.createRequest(0, 100, NewCursor(""), nil)

	// Verify that the proto files are included in the request
	require.NotNil(t, req)
	assert.Equal(t, pkg.ProtoFiles, req.ProtoFiles)
	assert.Len(t, req.ProtoFiles, 1)
	assert.Equal(t, "test.proto", req.ProtoFiles[0].GetName())
}

