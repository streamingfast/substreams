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
					{
						// A map field, which the descriptor carries as a repeated message
						// of a synthetic entry type.
						Name:     proto.String("balances"),
						Number:   proto.Int32(3),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_REPEATED.Enum(),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
						TypeName: proto.String(".test.output.Event.BalancesEntry"),
					},
					{
						Name:     proto.String("kind"),
						Number:   proto.Int32(4),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_ENUM.Enum(),
						TypeName: proto.String(".test.output.Event.Kind"),
					},
				},
				NestedType: []*descriptorpb.DescriptorProto{
					{
						Name:    proto.String("BalancesEntry"),
						Options: &descriptorpb.MessageOptions{MapEntry: proto.Bool(true)},
						Field: []*descriptorpb.FieldDescriptorProto{
							{
								Name:   proto.String("key"),
								Number: proto.Int32(1),
								Label:  descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
								Type:   descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
							},
							{
								Name:   proto.String("value"),
								Number: proto.Int32(2),
								Label:  descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
								Type:   descriptorpb.FieldDescriptorProto_TYPE_UINT64.Enum(),
							},
						},
					},
				},
				EnumType: []*descriptorpb.EnumDescriptorProto{
					{
						Name: proto.String("Kind"),
						Value: []*descriptorpb.EnumValueDescriptorProto{
							{Name: proto.String("KIND_UNSPECIFIED"), Number: proto.Int32(0)},
							{Name: proto.String("KIND_TRANSFER"), Number: proto.Int32(1)},
						},
					},
				},
			},
		},
	}

	rendered := renderProtoFile(file, "test.output.Event", true)

	require.Contains(t, rendered, `import "`+schemaProtoPath+`"`)
	require.Contains(t, rendered, "// option ("+schemaProtoPackage+".table)")
	require.Contains(t, rendered, "// string id = 1 [("+schemaProtoPackage+".field) = { primary_key: true }];")

	// A map field renders as a map, not as the entry type it is carried by, and the entry
	// type is not declared a second time.
	require.Contains(t, rendered, "map<string, uint64> balances = 3;")
	require.NotContains(t, rendered, "BalancesEntry")

	// A nested enum is declared where it is referenced from.
	require.Contains(t, rendered, "enum Kind {")

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
