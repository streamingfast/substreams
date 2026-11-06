package clickhouse

import (
	"testing"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/sink/sql/bytes"
	schema2 "github.com/streamingfast/substreams/sink/sql/db_proto/sql/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// TestUnannotatedTableGetsDefaultSortingKey covers the case a package with no
// schema.proto annotations lands in: the setup used to fail outright for want of
// 'order_by_fields', where it now sorts on the block number and a per-block row counter.
func TestUnannotatedTableGetsDefaultSortingKey(t *testing.T) {
	dialect := dialectFor(t, (&pbsubstreams.Clock{}).ProtoReflect().Descriptor())

	create := dialect.GetCreateTableSql("Clock")
	require.NotEmpty(t, create)

	assert.Contains(t, create, "_row_id_ UInt32 NOT NULL")
	assert.Contains(t, create, "PRIMARY KEY (_block_number_)")
	assert.Contains(t, create, "ORDER BY (_block_number_, _row_id_)")
	assert.Contains(t, create, "PARTITION BY (toYYYYMM(_block_timestamp_))")
	assert.True(t, dialect.UseRowIDField("Clock"))
}

// TestUseRowIDFieldIgnoresUnknownTable guards the walk, which asks about every message it
// meets, including the ones that never became a table.
func TestUseRowIDFieldIgnoresUnknownTable(t *testing.T) {
	dialect := dialectFor(t, (&pbsubstreams.Clock{}).ProtoReflect().Descriptor())

	assert.False(t, dialect.UseRowIDField("not_a_table"))
}

func dialectFor(t *testing.T, descriptor protoreflect.MessageDescriptor) *DialectClickHouse {
	t.Helper()

	schema, err := schema2.NewSchema("test", descriptor, false, zap.NewNop())
	require.NoError(t, err)

	dialect, err := NewDialectClickHouse(schema, bytes.EncodingRaw, zap.NewNop())
	require.NoError(t, err)

	return dialect
}
