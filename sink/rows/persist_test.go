package rows

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func relationsSchema(t *testing.T) *Schema {
	t.Helper()

	s, err := Resolve(relationsDescriptor())
	require.NoError(t, err)
	return s
}

func TestSchema_HashStable(t *testing.T) {
	s := relationsSchema(t)

	assert.Equal(t, s.Hash(), relationsSchema(t).Hash())
	assert.Len(t, s.Hash(), 32)

	// Table order does not matter.
	reordered := relationsSchema(t)
	reordered.Tables[0], reordered.Tables[1] = reordered.Tables[1], reordered.Tables[0]
	assert.Equal(t, s.Hash(), reordered.Hash())

	// A rename or a type change flips it.
	renamed := relationsSchema(t)
	renamed.Tables[0].Columns[0].Name = "other"
	assert.NotEqual(t, s.Hash(), renamed.Hash())

	retyped := relationsSchema(t)
	retyped.Table("types_tests").Column("str_2_decimal128").Type.Scale = 5
	assert.NotEqual(t, s.Hash(), retyped.Hash())
}

func TestSchema_Describe(t *testing.T) {
	var out bytes.Buffer
	require.NoError(t, relationsSchema(t).Describe(&out))

	text := out.String()
	assert.Contains(t, text, "schema "+relationsSchema(t).Hash())
	assert.Contains(t, text, "table orders (pk order_id)\n  order_id string pk\n  customer_ref_id string fk=customers.customer_id\n")
	assert.Contains(t, text, "table order_items (child_of orders on order_id)")
	assert.Contains(t, text, "  str_2_uint256 bigint(256,unsigned)")
	assert.Contains(t, text, "  repeated_int32_field list<int32>")
	assert.Contains(t, text, "  optional_string_set string optional")
}

func TestSchema_MarshalRoundTrip(t *testing.T) {
	s := relationsSchema(t)

	data, err := s.Marshal()
	require.NoError(t, err)

	loaded, err := UnmarshalSchema(data)
	require.NoError(t, err)

	assert.Equal(t, s.Hash(), loaded.Hash(), "names, types, keys and flags survive")
	assert.Equal(t, tableNames(s), tableNames(loaded))
	assert.Equal(t, s.Table("orders").PK, loaded.Table("orders").PK)
	assert.Equal(t, s.Table("order_items").ChildOf, loaded.Table("order_items").ChildOf)
	assert.True(t, loaded.Table("orders").Columns[0].PK)
	assert.Equal(t, s.Table("types_tests").Column("str_2_decimal128").Type.Scale, loaded.Table("types_tests").Column("str_2_decimal128").Type.Scale)
	assert.NoError(t, loaded.Compatible(s))
	assert.NoError(t, s.Compatible(loaded))

	_, err = UnmarshalSchema([]byte{0xff, 0xff})
	assert.Error(t, err)
}

func hand(tables ...Table) *Schema {
	s := &Schema{Tables: tables}
	for i := range s.Tables {
		s.Tables[i].ID = TableID(i)
		for k := range s.Tables[i].Columns {
			s.Tables[i].Columns[k].ID = ColumnID(k)
		}
	}
	s.index()
	return s
}

func col(name string, kind Kind) Column { return Column{Name: name, Type: LogicalType{Kind: kind}} }

func TestSchema_MergeIsAppendOnly(t *testing.T) {
	base := hand(Table{Name: "a", Columns: []Column{col("x", KindString)}})

	next := hand(
		Table{Name: "a", Columns: []Column{col("x", KindString), col("y", KindInt64)}},
		Table{Name: "b", Columns: []Column{col("z", KindBool)}},
	)

	changed, err := base.Merge(next)
	require.NoError(t, err)
	assert.True(t, changed)
	assert.EqualValues(t, 1, base.Version())
	assert.Equal(t, []string{"a", "b"}, tableNames(base))
	assert.EqualValues(t, 1, base.Table("b").ID)
	assert.EqualValues(t, 1, base.Table("a").Columns[1].ID)

	changed, err = base.Merge(next)
	require.NoError(t, err)
	assert.False(t, changed, "merging the same schema again changes nothing")
	assert.EqualValues(t, 1, base.Version())
}

func TestSchema_MergeRejectsIncompatible(t *testing.T) {
	base := hand(Table{Name: "a", Columns: []Column{col("x", KindString), col("y", KindInt64)}})

	tests := []struct {
		name string
		next *Schema
		want string
	}{
		{"removed table", hand(Table{Name: "other", Columns: []Column{col("x", KindString)}}), `table "a" was removed`},
		{"removed column", hand(Table{Name: "a", Columns: []Column{col("x", KindString)}}), "lost columns"},
		{"renamed or reordered column", hand(Table{Name: "a", Columns: []Column{col("y", KindInt64), col("x", KindString)}}), "append-only"},
		{"retyped column", hand(Table{Name: "a", Columns: []Column{col("x", KindString), col("y", KindString)}}), "changed type"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before := base.Hash()
			changed, err := base.Merge(tt.next)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
			assert.False(t, changed)
			assert.Equal(t, before, base.Hash(), "a rejected merge changes nothing")
		})
	}
}

func TestFileSchemaStore(t *testing.T) {
	store := NewFileSchemaStore(filepath.Join(t.TempDir(), "schema.pb"))

	loaded, err := store.Load(t.Context())
	require.NoError(t, err)
	assert.Nil(t, loaded, "nothing saved yet")

	s := relationsSchema(t)
	require.NoError(t, store.Save(t.Context(), s))

	loaded, err = store.Load(t.Context())
	require.NoError(t, err)
	require.NotNil(t, loaded)
	assert.Equal(t, s.Hash(), loaded.Hash())
}
