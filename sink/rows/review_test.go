package rows

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"

	pbschema "github.com/streamingfast/substreams/pb/sf/substreams/sink/schema/v1"
	schemapb "github.com/streamingfast/substreams/pb/sf/substreams/sink/v1"
)

func TestCompatible_KeyOrderAndFlags(t *testing.T) {
	base := func() *Schema {
		return hand(Table{Name: "a", Columns: []Column{col("x", KindString), col("y", KindString)}, PK: PrimaryKey{Kind: PKKindComposite, Composite: []ColumnID{0, 1}}})
	}

	reordered := base()
	reordered.Tables[0].PK.Composite = []ColumnID{1, 0}
	assert.Error(t, reordered.Compatible(base()), "same columns in another key order is a different key")

	optional := base()
	optional.Tables[0].Columns[0].Optional = true
	assert.Error(t, optional.Compatible(base()))

	unique := base()
	unique.Tables[0].Columns[1].Unique = true
	assert.Error(t, unique.Compatible(base()))

	childOf := base()
	childOf.Tables[0].HasChildOf, childOf.Tables[0].ChildOf = true, ChildOf{ParentTable: "p", ParentField: "id"}
	assert.Error(t, childOf.Compatible(base()))

	assert.NoError(t, base().Compatible(base()))
}

func TestMerge_DoesNotAliasNext(t *testing.T) {
	columns := make([]Column, 1, 8) // spare capacity: an aliasing append would write into it
	columns[0] = col("x", KindString)
	next := hand(Table{Name: "a", Columns: []Column{col("x", KindString)}}, Table{Name: "b", Columns: columns})

	s := hand(Table{Name: "a", Columns: []Column{col("x", KindString)}})
	changed, err := s.Merge(next)
	require.NoError(t, err)
	require.True(t, changed)

	grown := hand(Table{Name: "a", Columns: []Column{col("x", KindString)}}, Table{Name: "b", Columns: []Column{col("x", KindString), col("y", KindBool)}})
	changed, err = s.Merge(grown)
	require.NoError(t, err)
	require.True(t, changed)

	assert.Len(t, s.Table("b").Columns, 2)
	assert.Len(t, next.Tables[1].Columns, 1, "the merged-from schema is untouched")
	assert.Len(t, columns[:cap(columns)][1:2], 1)
	assert.Equal(t, Column{}, columns[:2][1], "nothing was written into next's backing array")
}

func TestUnmarshalSchema_RejectsInvalid(t *testing.T) {
	marshal := func(s *pbschema.Schema) []byte {
		data, err := s.MarshalVT()
		require.NoError(t, err)
		return data
	}
	typ := &pbschema.Schema_LogicalType{Kind: "string"}

	_, err := UnmarshalSchema(marshal(&pbschema.Schema{Tables: []*pbschema.Schema_Table{{Name: "a"}, {Name: "a"}}}))
	assert.ErrorContains(t, err, "duplicate table")

	_, err = UnmarshalSchema(marshal(&pbschema.Schema{Tables: []*pbschema.Schema_Table{{Name: "a", Columns: []*pbschema.Schema_Column{{Name: "x", Type: typ}, {Name: "x", Type: typ}}}}}))
	assert.ErrorContains(t, err, "duplicate column")

	_, err = UnmarshalSchema(marshal(&pbschema.Schema{Tables: []*pbschema.Schema_Table{{
		Name: "a", Columns: []*pbschema.Schema_Column{{Name: "x", Type: typ}},
		PrimaryKey: &pbschema.Schema_PrimaryKey{Key: &pbschema.Schema_PrimaryKey_Composite{Composite: &pbschema.Schema_CompositeKey{Columns: []uint32{0}}}},
	}}}))
	assert.ErrorContains(t, err, "at least 2 columns")
}

func TestResolve_ReviewCases(t *testing.T) {
	pk := &schemapb.Column{PrimaryKey: true}
	table := func(name string) *schemapb.Table { return &schemapb.Table{Name: name} }

	optionalID := stringField("id", 1, pk)
	optionalID.Proto3Optional = new(true)
	optionalID.OneofIndex = new(int32(0))

	listKey := &descriptorpb.FieldDescriptorProto{
		Name: new("ids"), Number: new(int32(1)), Type: descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
		Label: descriptorpb.FieldDescriptorProto_LABEL_REPEATED.Enum(), Options: fieldOptions(pk),
	}

	tests := []struct {
		name string
		msg  *descriptorpb.DescriptorProto
		more []*descriptorpb.DescriptorProto
		want string
	}{
		{
			name: "optional key", want: "is optional",
			msg: &descriptorpb.DescriptorProto{Name: new("Root"), Options: tableOptions(table("t")), Field: []*descriptorpb.FieldDescriptorProto{optionalID},
				OneofDecl: []*descriptorpb.OneofDescriptorProto{{Name: new("_id")}}},
		},
		{
			name: "list key", want: "a key must be a scalar",
			msg: &descriptorpb.DescriptorProto{Name: new("Root"), Options: tableOptions(table("t")), Field: []*descriptorpb.FieldDescriptorProto{listKey}},
		},
		{
			name: "empty primary_key_column", want: "primary_key_column is empty",
			msg: &descriptorpb.DescriptorProto{Name: new("Root"), Options: tableOptions(&schemapb.Table{Name: "t", PrimaryKey: &schemapb.Table_PrimaryKeyColumn{PrimaryKeyColumn: ""}}), Field: []*descriptorpb.FieldDescriptorProto{stringField("id", 1, nil)}},
		},
		{
			name: "child of itself", want: "child_of itself",
			msg: &descriptorpb.DescriptorProto{Name: new("Root"), Options: tableOptions(&schemapb.Table{Name: "t", ChildOf: new("t on id")}), Field: []*descriptorpb.FieldDescriptorProto{stringField("id", 1, pk)}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file := buildMessages(t, append([]*descriptorpb.DescriptorProto{tt.msg}, tt.more...)...)
			_, err := Resolve(file.Messages().ByName("Root"))
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

// child_of may name the parent key by its protobuf field name even when a (field).name override renames the column.
func TestResolve_ChildOfByFieldName(t *testing.T) {
	file := buildMessages(t,
		&descriptorpb.DescriptorProto{Name: new("Root"), Field: []*descriptorpb.FieldDescriptorProto{messageField("p", 1, ".test.resolve.Parent", false), messageField("k", 2, ".test.resolve.Kid", false)}},
		&descriptorpb.DescriptorProto{Name: new("Parent"), Options: tableOptions(&schemapb.Table{Name: "parents"}), Field: []*descriptorpb.FieldDescriptorProto{stringField("id", 1, &schemapb.Column{PrimaryKey: true, Name: new("pk")})}},
		&descriptorpb.DescriptorProto{Name: new("Kid"), Options: tableOptions(&schemapb.Table{Name: "kids", ChildOf: new("parents on id")}), Field: []*descriptorpb.FieldDescriptorProto{stringField("a", 1, nil)}},
	)
	_, err := Resolve(file.Messages().ByName("Root"))
	require.NoError(t, err)
}

// Well-known messages other than Timestamp (Duration, Any, Struct...) have neither column nor table, and must not fail
// the resolution, as a recursive google.protobuf.Struct would.
func TestResolve_IgnoresWellKnownMessages(t *testing.T) {
	fd, err := protodesc.NewFile(&descriptorpb.FileDescriptorProto{
		Name: new("test/wellknown.proto"), Package: new("test.wk"), Syntax: new("proto3"),
		Dependency: []string{"sf/substreams/sink/sql/schema/v1/schema.proto", "google/protobuf/struct.proto", "google/protobuf/duration.proto"},
		MessageType: []*descriptorpb.DescriptorProto{{
			Name: new("Row"), Options: tableOptions(&schemapb.Table{Name: "rows"}),
			Field: []*descriptorpb.FieldDescriptorProto{
				stringField("id", 1, &schemapb.Column{PrimaryKey: true}),
				messageField("meta", 2, ".google.protobuf.Struct", false),
				messageField("took", 3, ".google.protobuf.Duration", false),
			},
		}},
	}, protoregistry.GlobalFiles)
	require.NoError(t, err)

	s, err := Resolve(fd.Messages().ByName("Row"))
	require.NoError(t, err)
	require.Len(t, s.Tables[0].Columns, 1)
	assert.Equal(t, "id", s.Tables[0].Columns[0].Name)
}
