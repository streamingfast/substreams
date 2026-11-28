package rows

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"

	schemapb "github.com/streamingfast/substreams/pb/sf/substreams/sink/v1"
)

func tableNames(s *Schema) []string {
	names := make([]string, len(s.Tables))
	for i := range s.Tables {
		names[i] = s.Tables[i].Name
	}
	return names
}

func TestResolve_Relations(t *testing.T) {
	s, err := Resolve(relationsDescriptor())
	require.NoError(t, err)

	// Post-order discovery, as db_proto registers its tables: children before the table that holds them.
	assert.Equal(t, []string{"types_tests", "customers", "order_items", "order_extensions", "orders", "items"}, tableNames(s))
	for i := range s.Tables {
		assert.EqualValues(t, i, s.Tables[i].ID)
	}

	orders := s.Table("orders")
	require.NotNil(t, orders)
	assert.Equal(t, PrimaryKey{Kind: PKKindSingle, Single: 0}, orders.PK)
	assert.Equal(t, []string{"order_id", "customer_ref_id"}, []string{orders.Columns[0].Name, orders.Columns[1].Name})
	assert.Len(t, orders.Columns, 2, "items and extension are child tables, not columns")
	assert.True(t, orders.Columns[1].HasFK)
	assert.Equal(t, ForeignKey{Table: "customers", Field: "customer_id"}, orders.Columns[1].FK)
	assert.ElementsMatch(t, []TableID{s.Table("order_items").ID, s.Table("order_extensions").ID}, orders.Children)

	items := s.Table("order_items")
	assert.True(t, items.HasChildOf)
	assert.Equal(t, ChildOf{ParentTable: "orders", ParentField: "order_id"}, items.ChildOf)
	assert.Equal(t, PKKindNone, items.PK.Kind)

	assert.True(t, s.Table("items").Column("item_id").Unique)
	assert.Nil(t, s.Table("nope"))
}

func TestResolve_ColumnTypes(t *testing.T) {
	s, err := Resolve(relationsDescriptor())
	require.NoError(t, err)

	types := s.Table("types_tests")
	require.NotNil(t, types)

	tests := []struct {
		column   string
		want     LogicalType
		optional bool
	}{
		{"id", LogicalType{Kind: KindUint64}, false},
		{"double_field", LogicalType{Kind: KindFloat64}, false},
		{"float_field", LogicalType{Kind: KindFloat32}, false},
		{"sint32_field", LogicalType{Kind: KindInt32}, false},
		{"sfixed64_field", LogicalType{Kind: KindInt64}, false},
		{"fixed32_field", LogicalType{Kind: KindUint32}, false},
		{"bool_field", LogicalType{Kind: KindBool}, false},
		{"bytes_field", LogicalType{Kind: KindBytes}, false},
		{"optional_string_set", LogicalType{Kind: KindString}, true},
		{"timestamp_field", LogicalType{Kind: KindTimestamp}, true},
		{"repeated_int32_field", LogicalType{Kind: KindList, Elem: KindInt32}, false},
		{"str_2_int128", LogicalType{Kind: KindBigint, Bits: 128, Signed: true}, false},
		{"str_2_uint256", LogicalType{Kind: KindBigint, Bits: 256}, false},
		{"str_2_decimal128", LogicalType{Kind: KindDecimal, Bits: 128, Scale: 4}, false},
		{"str_2_decimal256", LogicalType{Kind: KindDecimal, Bits: 256, Scale: 4}, false},
		{"optional_str_2_uint256", LogicalType{Kind: KindBigint, Bits: 256}, true},
	}
	for _, tt := range tests {
		t.Run(tt.column, func(t *testing.T) {
			column := types.Column(tt.column)
			require.NotNil(t, column)
			assert.Equal(t, tt.want, column.Type)
			assert.Equal(t, tt.optional, column.Optional)
		})
	}

	level1 := types.Column("level1")
	require.NotNil(t, level1)
	assert.Equal(t, KindStruct, level1.Type.Kind)
	assert.Equal(t, protoreflect.FullName("test.relations.NestedLevel1"), level1.Type.Struct.FullName())

	list := types.Column("list_of_level1")
	require.NotNil(t, list)
	assert.Equal(t, KindList, list.Type.Kind)
	assert.Equal(t, KindStruct, list.Type.Elem)
	assert.Equal(t, KindStruct, list.Type.ElemType().Kind)

	assert.Equal(t, "bigint(256,unsigned)", types.Column("str_2_uint256").Type.String())
	assert.Equal(t, "decimal(128,4)", types.Column("str_2_decimal128").Type.String())
	assert.Equal(t, "list<int32>", types.Column("repeated_int32_field").Type.String())
}

// buildMessages builds descriptors from a file with the given messages, resolving the schema options from the global registry.
func buildMessages(t *testing.T, messages ...*descriptorpb.DescriptorProto) protoreflect.FileDescriptor {
	t.Helper()

	fd, err := protodesc.NewFile(&descriptorpb.FileDescriptorProto{
		Name:        new("test/resolve.proto"),
		Package:     new("test.resolve"),
		Syntax:      new("proto3"),
		Dependency:  []string{"sf/substreams/sink/sql/schema/v1/schema.proto"},
		MessageType: messages,
	}, protoregistry.GlobalFiles)
	require.NoError(t, err)
	return fd
}

func tableOptions(opts *schemapb.Table) *descriptorpb.MessageOptions {
	options := &descriptorpb.MessageOptions{}
	proto.SetExtension(options, schemapb.E_Table, opts)
	return options
}

func fieldOptions(opts *schemapb.Column) *descriptorpb.FieldOptions {
	options := &descriptorpb.FieldOptions{}
	proto.SetExtension(options, schemapb.E_Field, opts)
	return options
}

func stringField(name string, number int32, opts *schemapb.Column) *descriptorpb.FieldDescriptorProto {
	field := &descriptorpb.FieldDescriptorProto{
		Name: &name, Number: &number,
		Type:  descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
		Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
	}
	if opts != nil {
		field.Options = fieldOptions(opts)
	}
	return field
}

func messageField(name string, number int32, typeName string, repeated bool) *descriptorpb.FieldDescriptorProto {
	label := descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL
	if repeated {
		label = descriptorpb.FieldDescriptorProto_LABEL_REPEATED
	}
	return &descriptorpb.FieldDescriptorProto{
		Name: &name, Number: &number, TypeName: &typeName,
		Type:  descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
		Label: &label,
	}
}

func TestResolve_Errors(t *testing.T) {
	pk := &schemapb.Column{PrimaryKey: true}
	table := func(name string) *schemapb.Table { return &schemapb.Table{Name: name} }

	tests := []struct {
		name     string
		messages []*descriptorpb.DescriptorProto
		root     string
		want     string
	}{
		{
			name: "recursive type",
			root: "Node",
			messages: []*descriptorpb.DescriptorProto{
				{Name: new("Node"), Options: tableOptions(table("nodes")), Field: []*descriptorpb.FieldDescriptorProto{stringField("id", 1, pk), messageField("next", 2, ".test.resolve.Node", false)}},
			},
			want: "recursive message type",
		},
		{
			name: "no table",
			root: "Plain",
			messages: []*descriptorpb.DescriptorProto{
				{Name: new("Plain"), Field: []*descriptorpb.FieldDescriptorProto{stringField("id", 1, nil)}},
			},
			want: "no table found",
		},
		{
			name: "convertTo on a non string",
			root: "Bad",
			messages: []*descriptorpb.DescriptorProto{
				{Name: new("Bad"), Options: tableOptions(table("bad")), Field: []*descriptorpb.FieldDescriptorProto{
					{Name: new("n"), Number: new(int32(1)), Type: descriptorpb.FieldDescriptorProto_TYPE_INT64.Enum(), Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
						Options: fieldOptions(&schemapb.Column{ConvertTo: &schemapb.StringConvertion{Convertion: &schemapb.StringConvertion_Int128{Int128: &schemapb.Int128{}}}})},
				}},
			},
			want: "convertTo is only valid on string fields",
		},
		{
			name: "decimal scale out of range",
			root: "Bad",
			messages: []*descriptorpb.DescriptorProto{
				{Name: new("Bad"), Options: tableOptions(table("bad")), Field: []*descriptorpb.FieldDescriptorProto{
					stringField("d", 1, &schemapb.Column{ConvertTo: &schemapb.StringConvertion{Convertion: &schemapb.StringConvertion_Decimal128{Decimal128: &schemapb.Decimal128{Scale: 39}}}}),
				}},
			},
			want: "scale 39 out of range",
		},
		{
			name: "duplicate column names",
			root: "Dup",
			messages: []*descriptorpb.DescriptorProto{
				{Name: new("Dup"), Options: tableOptions(table("dup")), Field: []*descriptorpb.FieldDescriptorProto{
					stringField("a", 1, &schemapb.Column{Name: new("x")}), stringField("b", 2, &schemapb.Column{Name: new("x")}),
				}},
			},
			want: `duplicate column name "x"`,
		},
		{
			name: "child of an unknown table",
			root: "Child",
			messages: []*descriptorpb.DescriptorProto{
				{Name: new("Child"), Options: tableOptions(&schemapb.Table{Name: "child", ChildOf: new("ghosts on id")}), Field: []*descriptorpb.FieldDescriptorProto{stringField("a", 1, nil)}},
			},
			want: `child_of unknown table "ghosts"`,
		},
		{
			name: "child of a parent without key",
			root: "Root",
			messages: []*descriptorpb.DescriptorProto{
				{Name: new("Root"), Field: []*descriptorpb.FieldDescriptorProto{messageField("parent", 1, ".test.resolve.Parent", false), messageField("kid", 2, ".test.resolve.Kid", false)}},
				{Name: new("Parent"), Options: tableOptions(table("parents")), Field: []*descriptorpb.FieldDescriptorProto{stringField("id", 1, nil)}},
				{Name: new("Kid"), Options: tableOptions(&schemapb.Table{Name: "kids", ChildOf: new("parents on id")}), Field: []*descriptorpb.FieldDescriptorProto{stringField("a", 1, nil)}},
			},
			want: "no single-column primary key",
		},
		{
			name: "child of the wrong parent field",
			root: "Root",
			messages: []*descriptorpb.DescriptorProto{
				{Name: new("Root"), Field: []*descriptorpb.FieldDescriptorProto{messageField("parent", 1, ".test.resolve.Parent", false), messageField("kid", 2, ".test.resolve.Kid", false)}},
				{Name: new("Parent"), Options: tableOptions(table("parents")), Field: []*descriptorpb.FieldDescriptorProto{stringField("id", 1, pk)}},
				{Name: new("Kid"), Options: tableOptions(&schemapb.Table{Name: "kids", ChildOf: new("parents on other")}), Field: []*descriptorpb.FieldDescriptorProto{stringField("a", 1, nil)}},
			},
			want: `the parent's primary key is "id"`,
		},
		{
			name: "same table name on two messages",
			root: "Root",
			messages: []*descriptorpb.DescriptorProto{
				{Name: new("Root"), Field: []*descriptorpb.FieldDescriptorProto{messageField("a", 1, ".test.resolve.A", false), messageField("b", 2, ".test.resolve.B", false)}},
				{Name: new("A"), Options: tableOptions(table("same")), Field: []*descriptorpb.FieldDescriptorProto{stringField("x", 1, nil)}},
				{Name: new("B"), Options: tableOptions(table("same")), Field: []*descriptorpb.FieldDescriptorProto{stringField("x", 1, nil)}},
			},
			want: `table name "same" is used by messages`,
		},
		{
			name: "invalid child_of format",
			root: "Child",
			messages: []*descriptorpb.DescriptorProto{
				{Name: new("Child"), Options: tableOptions(&schemapb.Table{Name: "child", ChildOf: new("nonsense")}), Field: []*descriptorpb.FieldDescriptorProto{stringField("a", 1, nil)}},
			},
			want: "expecting 'table_name on field_name'",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file := buildMessages(t, tt.messages...)
			_, err := Resolve(file.Messages().ByName(protoreflect.Name(tt.root)))
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

func TestResolve_CompositeKey(t *testing.T) {
	file := buildMessages(t, &descriptorpb.DescriptorProto{
		Name: new("Edge"), Options: tableOptions(&schemapb.Table{Name: "edges"}),
		Field: []*descriptorpb.FieldDescriptorProto{
			stringField("from", 1, &schemapb.Column{PrimaryKey: true}),
			stringField("label", 2, nil),
			stringField("to", 3, &schemapb.Column{PrimaryKey: true}),
		},
	})

	s, err := Resolve(file.Messages().ByName("Edge"))
	require.NoError(t, err)

	pk := s.Table("edges").PK
	assert.Equal(t, PKKindComposite, pk.Kind)
	assert.Equal(t, []ColumnID{0, 2}, pk.Composite)
	assert.Equal(t, 2, pk.Len())
	assert.Equal(t, ColumnID(2), pk.At(1))
}

func TestResolve_NoAnnotationsRequired(t *testing.T) {
	file := buildMessages(t,
		&descriptorpb.DescriptorProto{Name: new("Root"), Field: []*descriptorpb.FieldDescriptorProto{messageField("kids", 1, ".test.resolve.Kid", true), stringField("name", 2, nil)}},
		&descriptorpb.DescriptorProto{Name: new("Kid"), Field: []*descriptorpb.FieldDescriptorProto{stringField("a", 1, nil)}},
	)

	s, err := Resolve(file.Messages().ByName("Root"), WithRequireAnnotations(false))
	require.NoError(t, err)
	assert.Equal(t, []string{"Kid", "Root"}, tableNames(s), "every message with columns is a table named after the message")
}

func TestHasTableSchema(t *testing.T) {
	assert.True(t, HasTableSchema(relationsDescriptor()), "an output wrapping annotated messages")
	assert.False(t, HasTableSchema(nil))

	file := buildMessages(t, &descriptorpb.DescriptorProto{Name: new("Plain"), Field: []*descriptorpb.FieldDescriptorProto{stringField("a", 1, nil)}})
	assert.False(t, HasTableSchema(file.Messages().ByName("Plain")))
}

func TestResolve_TableLevelPrimaryKey(t *testing.T) {
	edge := func(info *schemapb.Table, fields ...*descriptorpb.FieldDescriptorProto) protoreflect.MessageDescriptor {
		info.Name = "edges"
		file := buildMessages(t, &descriptorpb.DescriptorProto{Name: new("Edge"), Options: tableOptions(info), Field: fields})
		return file.Messages().ByName("Edge")
	}
	fields := func(flagged ...string) []*descriptorpb.FieldDescriptorProto {
		out := []*descriptorpb.FieldDescriptorProto{stringField("from", 1, nil), stringField("to", 2, nil), stringField("label", 3, nil)}
		for _, name := range flagged {
			for _, f := range out {
				if f.GetName() == name {
					f.Options = fieldOptions(&schemapb.Column{PrimaryKey: true})
				}
			}
		}
		return out
	}

	t.Run("single column", func(t *testing.T) {
		s, err := Resolve(edge(&schemapb.Table{PrimaryKey: &schemapb.Table_PrimaryKeyColumn{PrimaryKeyColumn: "from"}}, fields()...))
		require.NoError(t, err)
		assert.Equal(t, PrimaryKey{Kind: PKKindSingle, Single: 0}, s.Table("edges").PK)
		assert.True(t, s.Table("edges").Columns[0].PK)
	})

	t.Run("composite in declared order", func(t *testing.T) {
		composite := &schemapb.Table_CompositePrimaryKey{CompositePrimaryKey: &schemapb.CompositeKey{Columns: []string{"to", "from"}}}
		s, err := Resolve(edge(&schemapb.Table{PrimaryKey: composite}, fields()...))
		require.NoError(t, err)
		assert.Equal(t, PrimaryKey{Kind: PKKindComposite, Composite: []ColumnID{1, 0}}, s.Table("edges").PK, "key order is the declared one, not the field order")
	})

	t.Run("agrees with flagged fields", func(t *testing.T) {
		s, err := Resolve(edge(&schemapb.Table{PrimaryKey: &schemapb.Table_PrimaryKeyColumn{PrimaryKeyColumn: "from"}}, fields("from")...))
		require.NoError(t, err)
		assert.Equal(t, PKKindSingle, s.Table("edges").PK.Kind)
	})

	t.Run("flagged field outside the declared key", func(t *testing.T) {
		_, err := Resolve(edge(&schemapb.Table{PrimaryKey: &schemapb.Table_PrimaryKeyColumn{PrimaryKeyColumn: "from"}}, fields("label")...))
		require.Error(t, err)
		assert.Contains(t, err.Error(), `column "label" is flagged primary_key`)
	})

	t.Run("unknown column", func(t *testing.T) {
		_, err := Resolve(edge(&schemapb.Table{PrimaryKey: &schemapb.Table_PrimaryKeyColumn{PrimaryKeyColumn: "ghost"}}, fields()...))
		require.Error(t, err)
		assert.Contains(t, err.Error(), `primary key column "ghost" not found`)
	})

	t.Run("composite of one column", func(t *testing.T) {
		composite := &schemapb.Table_CompositePrimaryKey{CompositePrimaryKey: &schemapb.CompositeKey{Columns: []string{"from"}}}
		_, err := Resolve(edge(&schemapb.Table{PrimaryKey: composite}, fields()...))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "needs at least 2 columns")
	})
}
