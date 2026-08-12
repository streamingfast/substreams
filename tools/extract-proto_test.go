package tools

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jhump/protoreflect/desc/protoparse"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
)

// TestRenderProtoFileParsesWithAnnotations is the property that makes the command worth
// having: what it writes has to parse once the operator uncomments an option, against the
// annotations file it wrote beside it. A scaffold that does not compile is worse than no
// scaffold.
func TestRenderProtoFileParsesWithAnnotations(t *testing.T) {
	file := &descriptorpb.FileDescriptorProto{
		Name:    proto.String("events.proto"),
		Package: proto.String("test.output"),
		Syntax:  proto.String("proto3"),
		MessageType: []*descriptorpb.DescriptorProto{
			{
				Name: proto.String("Event"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{
						Name:   proto.String("id"),
						Number: proto.Int32(1),
						Label:  descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
						Type:   descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
					},
					{
						Name:   proto.String("amount"),
						Number: proto.Int32(2),
						Label:  descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
						Type:   descriptorpb.FieldDescriptorProto_TYPE_UINT64.Enum(),
					},
				},
			},
		},
	}

	rendered := renderProtoFile(file, "test.output.Event", true)

	require.Contains(t, rendered, `import "`+schemaProtoPath+`"`)
	require.Contains(t, rendered, "// option ("+schemaProtoPackage+".table)")
	require.Contains(t, rendered, "// [("+schemaProtoPackage+".field) = { primary_key: true }]")

	// Uncomment what an operator would, then require the result to parse.
	rendered = strings.Replace(rendered, `  // option (schema.table) = { name: "event" };`, `  option (schema.table) = { name: "event" };`, 1)
	rendered = strings.Replace(rendered, "  string id = 1;", "  string id = 1 [(schema.field) = { primary_key: true }];", 1)

	directory := t.TempDir()
	require.NoError(t, writeFile(filepath.Join(directory, "events.proto"), rendered))
	require.NoError(t, writeFile(filepath.Join(directory, schemaProtoPath), mustReadSchemaProto(t)))

	parser := protoparse.Parser{ImportPaths: []string{directory}}
	fds, err := parser.ParseFiles("events.proto")
	require.NoError(t, err, "the scaffold has to parse once its options are uncommented")
	require.Len(t, fds[0].GetMessageTypes(), 1)
}

func mustReadSchemaProto(t *testing.T) string {
	t.Helper()

	content, err := os.ReadFile(filepath.Join("..", "proto", schemaProtoPath))
	require.NoError(t, err)

	return string(content)
}
