package clickhouse

import (
	"testing"

	sql2 "github.com/streamingfast/substreams/sink/sql/db_proto/sql"
	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// TestInt32ValueAcceptsEveryEnumRendering covers the three shapes that can reach an Int32
// column: the walk's own wrapper, a raw protoreflect enum from a path that skipped the
// walk, and a plain int32 from a non-enum field.
func TestInt32ValueAcceptsEveryEnumRendering(t *testing.T) {
	assert.Equal(t, int32(2), int32Value(sql2.EnumValue{Number: 2, Name: "UPDATE_POLICY_SET"}))
	assert.Equal(t, int32(3), int32Value(protoreflect.EnumNumber(3)))
	assert.Equal(t, int32(-7), int32Value(int32(-7)))
}

// TestInt32ValuePanicsOnUnrelatedTypes keeps the helper from silently swallowing a value
// the column was never meant to hold.
func TestInt32ValuePanicsOnUnrelatedTypes(t *testing.T) {
	assert.Panics(t, func() { int32Value("not a number") })
	assert.Panics(t, func() { int32Value(int64(4)) })
}
