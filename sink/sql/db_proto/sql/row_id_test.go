package sql

import (
	"testing"
	"time"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/sink/sql/db_proto/sql/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"
)

// TestWalkNumbersRowsPerBlock checks the counter the ClickHouse sorting key relies on:
// every row a block writes to a table gets the next number, starting over at zero for the
// next block. Without it the ReplacingMergeTree collapses a block's rows into one.
func TestWalkNumbersRowsPerBlock(t *testing.T) {
	message := &pbsubstreams.Modules{
		Modules: []*pbsubstreams.Module{
			{Name: "map_one"},
			{Name: "map_two"},
			{Name: "map_three"},
		},
	}

	inserter := &recordingInserter{}
	database := databaseFor(t, message.ProtoReflect().Descriptor())

	dynamic := dynamicFor(t, message)
	_, err := database.WalkMessageDescriptorAndInsertWithDialect(dynamic, 100, time.Unix(0, 0), nil, rowIDDialect{}, inserter)
	require.NoError(t, err)

	// Row 0 is the Modules message itself, then one per module.
	assert.Equal(t, []uint32{0}, inserter.rowIDs["Modules"])
	assert.Equal(t, []uint32{0, 1, 2}, inserter.rowIDs["Module"])

	// A second block starts its own numbering rather than continuing the first one's.
	inserter.rowIDs = nil
	_, err = database.WalkMessageDescriptorAndInsertWithDialect(dynamic, 101, time.Unix(0, 0), nil, rowIDDialect{}, inserter)
	require.NoError(t, err)

	assert.Equal(t, []uint32{0, 1, 2}, inserter.rowIDs["Module"])
}

// TestWalkOmitsRowIDWhenDialectDeclinesIt is the PostgreSQL side: nothing extra is
// appended, so the column count the inserters expect is unchanged.
func TestWalkOmitsRowIDWhenDialectDeclinesIt(t *testing.T) {
	message := &pbsubstreams.Modules{Modules: []*pbsubstreams.Module{{Name: "map_one"}}}

	inserter := &recordingInserter{}
	database := databaseFor(t, message.ProtoReflect().Descriptor())

	_, err := database.WalkMessageDescriptorAndInsertWithDialect(dynamicFor(t, message), 100, time.Unix(0, 0), nil, plainDialect{}, inserter)
	require.NoError(t, err)

	for table, rows := range inserter.values {
		for _, values := range rows {
			// blockNum, blockTimestamp and nothing else injected.
			require.GreaterOrEqual(t, len(values), 2, "table %q", table)
			if len(values) < 3 {
				continue
			}
			_, isRowID := values[2].(uint32)
			assert.False(t, isRowID, "table %q got a row id", table)
		}
	}
}

func databaseFor(t *testing.T, descriptor protoreflect.MessageDescriptor) *BaseDatabase {
	t.Helper()

	database, err := NewBaseDatabase("test", descriptor, false, zap.NewNop())
	require.NoError(t, err)

	return database
}

func dynamicFor(t *testing.T, message proto.Message) protoreflect.Message {
	t.Helper()

	encoded, err := proto.Marshal(message)
	require.NoError(t, err)

	dynamic := dynamicpb.NewMessage(message.ProtoReflect().Descriptor())
	require.NoError(t, proto.Unmarshal(encoded, dynamic))

	return dynamic
}

type recordingInserter struct {
	rowIDs map[string][]uint32
	values map[string][][]any
}

func (i *recordingInserter) Insert(table string, values []any) error {
	if i.values == nil {
		i.values = map[string][][]any{}
	}
	i.values[table] = append(i.values[table], values)

	if len(values) < 3 {
		return nil
	}

	if rowID, ok := values[2].(uint32); ok {
		if i.rowIDs == nil {
			i.rowIDs = map[string][]uint32{}
		}
		i.rowIDs[table] = append(i.rowIDs[table], rowID)
	}

	return nil
}

// rowIDDialect keeps every message it is asked about, with the row id column on, and no
// version or deleted column so the id sits at a fixed index.
type rowIDDialect struct{ plainDialect }

func (rowIDDialect) UseRowIDField(string) bool { return true }

type plainDialect struct{}

func (plainDialect) SchemaHash() string                            { return "" }
func (plainDialect) FullTableName(table *schema.Table) string      { return table.Name }
func (plainDialect) GetTable(table string) *schema.Table           { return &schema.Table{Name: table} }
func (plainDialect) GetTables() []*schema.Table                    { return nil }
func (plainDialect) UseVersionField() bool                         { return false }
func (plainDialect) UseDeletedField() bool                         { return false }
func (plainDialect) UseRowIDField(string) bool                     { return false }
func (plainDialect) AppendInlineFieldValues(fieldValues []any, fd protoreflect.FieldDescriptor, fv protoreflect.Value, dm protoreflect.Message) ([]any, error) {
	return fieldValues, nil
}
