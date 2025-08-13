package sink

import (
	"testing"

	"github.com/stretchr/testify/assert"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"google.golang.org/protobuf/types/descriptorpb"
)

func TestRequestIncludesProtoFiles(t *testing.T) {
	// Create a mock package with proto files
	pkg := &pbsubstreams.Package{
		ProtoFiles: []*descriptorpb.FileDescriptorProto{
			{
				Name:    strPtr("test.proto"),
				Package: strPtr("test"),
			},
		},
	}

	// Create a request with the proto files
	req := &pbsubstreams.Request{
		ProtoFiles: pkg.ProtoFiles,
	}

	// Verify that the proto files are included in the request
	assert.NotNil(t, req.ProtoFiles)
	assert.Len(t, req.ProtoFiles, 1)
	assert.Equal(t, "test.proto", req.ProtoFiles[0].GetName())
	assert.Equal(t, "test", req.ProtoFiles[0].GetPackage())
}

func strPtr(s string) *string {
	return &s
}

