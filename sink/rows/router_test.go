package rows

import (
	"testing"

	"google.golang.org/protobuf/types/descriptorpb"

	schemapb "github.com/streamingfast/substreams/pb/sf/substreams/sink/v1"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/streamingfast/substreams/sink/sql/tests/relations"
)

func TestDetectKind(t *testing.T) {
	plain := (&relations.NestedLevel1{}).ProtoReflect().Descriptor()

	assert.Equal(t, RowProducerKindDatabaseChanges, DetectRowProducerKind("proto:sf.substreams.sink.database.v1.DatabaseChanges", nil))
	assert.Equal(t, RowProducerKindDatabaseChanges, DetectRowProducerKind("sf.substreams.database.v1.DatabaseChanges", nil))
	assert.Equal(t, RowProducerKindSchema, DetectRowProducerKind("proto:test.relations.Output", relationsDescriptor()))
	assert.Equal(t, RowProducerKindBlock, DetectRowProducerKind("proto:test.relations.NestedLevel1", plain))
	assert.Equal(t, RowProducerKindBlock, DetectRowProducerKind("proto:acme.Unknown", nil))
}

func TestNewProducer(t *testing.T) {
	p, err := NewRowProducer(RowProducerKindAuto, "proto:test.relations.Output", relationsDescriptor())
	require.NoError(t, err)
	assert.IsType(t, &SchemaRowProducer{}, p)

	p, err = NewRowProducer(RowProducerKindAuto, "proto:test.relations.NestedLevel1", (&relations.NestedLevel1{}).ProtoReflect().Descriptor(), WithModuleName("map_nested"))
	require.NoError(t, err)
	assert.IsType(t, &BlockRowProducer{}, p)
	assert.Equal(t, "map_nested", p.Schema().Tables[0].Name)

	// An explicit kind overrides detection.
	p, err = NewRowProducer(RowProducerKindBlock, "proto:test.relations.Output", relationsDescriptor())
	require.NoError(t, err)
	assert.IsType(t, &BlockRowProducer{}, p)

	_, err = NewRowProducer(RowProducerKindAuto, "proto:sf.substreams.sink.database.v1.DatabaseChanges", nil)
	require.ErrorIs(t, err, ErrRowProducerUnavailable)

	_, err = NewRowProducer(RowProducerKindSchema, "x", nil)
	require.Error(t, err)

	_, err = NewRowProducer(RowProducerKindSchema, "proto:test.relations.NestedLevel1", (&relations.NestedLevel1{}).ProtoReflect().Descriptor())
	require.Error(t, err, "no table annotation")
}

func TestBlockProducer(t *testing.T) {
	out := sampleOutput()
	p, err := NewBlockRowProducer(out.ProtoReflect().Descriptor(), WithModuleName("map_out"))
	require.NoError(t, err)

	for _, f := range factories() {
		t.Run(f.name, func(t *testing.T) {
			count := 0
			for row, err := range p.Rows(testBlock, f.make(t, out)) {
				require.NoError(t, err)
				count++

				assert.Equal(t, "map_out", row.Table.Name)
				assert.Equal(t, BlockValueColumn, row.Table.Columns[0].Name)
				assert.Equal(t, KindStruct, row.Table.Columns[0].Type.Kind)
				value, ok := row.Value(0)
				require.True(t, ok)
				assert.Equal(t, 4, value.Message().Get(value.Message().Descriptor().Fields().ByName("entities")).List().Len())
				assert.EqualValues(t, 100, row.Block.Number)
				assert.EqualValues(t, 0, row.Index)
				assert.EqualValues(t, 0, row.SeqBlock)
			}
			assert.Equal(t, 1, count, "exactly one row per block")
		})
	}

	for _, err := range p.Rows(testBlock, nil) {
		require.Error(t, err)
	}
}

func TestRequireStatic(t *testing.T) {
	p := newRelationsProducer(t)
	require.NoError(t, RequireStatic(p))

	assert.ErrorIs(t, RequireStatic(discovering{p}), ErrDiscoveredSchema)
}

type discovering struct{ RowProducer }

func (discovering) SchemaVersion() uint64 { return 1 }

// HasTableSchema follows the same fields Resolve does: a table only reachable through a map value or a well-known message does not count.
func TestHasTableSchema_FollowsResolveFields(t *testing.T) {
	annotated := &descriptorpb.DescriptorProto{Name: new("Annotated"), Options: tableOptions(&schemapb.Table{Name: "annotated"}), Field: []*descriptorpb.FieldDescriptorProto{stringField("id", 1, nil)}}

	mapEntry := &descriptorpb.DescriptorProto{
		Name:    new("Root"),
		Field:   []*descriptorpb.FieldDescriptorProto{messageField("by_key", 1, ".test.resolve.Root.ByKeyEntry", true)},
		Options: nil,
		NestedType: []*descriptorpb.DescriptorProto{{
			Name:    new("ByKeyEntry"),
			Options: &descriptorpb.MessageOptions{MapEntry: new(true)},
			Field:   []*descriptorpb.FieldDescriptorProto{stringField("key", 1, nil), messageField("value", 2, ".test.resolve.Annotated", false)},
		}},
	}
	viaMap := buildMessages(t, annotated, mapEntry).Messages().ByName("Root")
	assert.False(t, HasTableSchema(viaMap), "a table reachable only through a map value is not resolved, so it must not be detected")

	direct := &descriptorpb.DescriptorProto{Name: new("Direct"), Field: []*descriptorpb.FieldDescriptorProto{messageField("a", 1, ".test.resolve.Annotated2", false)}}
	annotated2 := &descriptorpb.DescriptorProto{Name: new("Annotated2"), Options: tableOptions(&schemapb.Table{Name: "annotated2"}), Field: []*descriptorpb.FieldDescriptorProto{stringField("id", 1, nil)}}
	assert.True(t, HasTableSchema(buildMessages(t, annotated2, direct).Messages().ByName("Direct")))
}
