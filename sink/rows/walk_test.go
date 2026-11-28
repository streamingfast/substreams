package rows

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/dynamicpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/streamingfast/substreams/sink/sql/tests/relations"
)

func newRelationsProducer(t *testing.T, opts ...Option) *SchemaRowProducer {
	t.Helper()

	p, err := NewSchemaRowProducer(relationsDescriptor(), opts...)
	require.NoError(t, err)
	return p
}

var testBlock = BlockRef{Number: 100, ID: "0xabc", Timestamp: time.Unix(1700000000, 0).UTC()}

func TestWalk_PreOrder(t *testing.T) {
	p := newRelationsProducer(t)

	for _, f := range factories() {
		t.Run(f.name, func(t *testing.T) {
			rows := collect(t, p, testBlock, f.make(t, sampleOutput()))

			type position struct {
				Table  string
				Index  uint32
				Parent string
			}
			var got []position
			for i, row := range rows {
				got = append(got, position{row.Table, row.Index, row.Parent})
				assert.EqualValues(t, i, row.SeqBlock, "block-wide position follows emission order")
			}

			// Entity is a pass-through message: its rows are top level. A parent comes before its children, in field order.
			assert.Equal(t, []position{
				{"customers", 0, ""},
				{"orders", 0, ""},
				{"order_items", 0, "orders"},
				{"order_items", 1, "orders"},
				{"order_extensions", 0, "orders"},
				{"types_tests", 0, ""},
				{"items", 0, ""},
			}, got)

			assert.Equal(t, "c1", rows[0].Values["customer_id"])
			assert.Equal(t, "i2", rows[3].Values["item_id"])
			assert.EqualValues(t, 3, rows[3].Values["quantity"])
		})
	}
}

func TestWalk_PresentAndValues(t *testing.T) {
	p := newRelationsProducer(t)
	out := sampleOutput()
	out.Entities[2].GetTypesTest().TimestampField = timestamppb.New(time.Unix(1700000000, 0))

	for _, f := range factories() {
		t.Run(f.name, func(t *testing.T) {
			var types snapshot
			for _, row := range collect(t, p, testBlock, f.make(t, out)) {
				if row.Table == "types_tests" {
					types = row
				}
			}

			assert.EqualValues(t, 7, types.Values["id"])
			assert.Equal(t, "hello", types.Values["string_field"])
			assert.Equal(t, "\x01\x02", types.Values["bytes_field"])
			assert.Equal(t, "set", types.Values["optional_string_set"])
			assert.EqualValues(t, 5, types.Values["optional_int32_field_set"])
			assert.Equal(t, []any{int64(1), int64(2), int64(3)}, types.Values["repeated_int32_field"])
			assert.Equal(t, "12.3456", types.Values["str_2_decimal128"])
			assert.Equal(t, time.Unix(1700000000, 0).UTC(), types.Values["timestamp_field"])

			_, hasUnset := types.Values["optional_string_not_set"]
			assert.False(t, hasUnset, "an unset optional is absent")
			_, hasUnsetInt := types.Values["optional_int32_field_not_set"]
			assert.False(t, hasUnsetInt)
			assert.Equal(t, "struct", types.Values["level1"])

			zero, hasZero := types.Values["double_field"]
			assert.True(t, hasZero, "an implicit proto3 scalar is present with its zero value")
			assert.EqualValues(t, 0, zero)
			assert.Equal(t, []any{}, types.Values["repeated_string_field"], "an empty list is present")
			assert.Equal(t, []any{}, types.Values["list_of_level1"])
		})
	}
}

func TestWalk_TimestampUnset(t *testing.T) {
	p := newRelationsProducer(t)
	for _, f := range factories() {
		t.Run(f.name, func(t *testing.T) {
			for _, row := range collect(t, p, testBlock, f.make(t, sampleOutput())) {
				if row.Table == "types_tests" {
					_, has := row.Values["timestamp_field"]
					assert.False(t, has, "an unset Timestamp is absent")
				}
			}
		})
	}
}

// TestWalk_Equivalence: every message implementation yields the same rows.
func TestWalk_Equivalence(t *testing.T) {
	p := newRelationsProducer(t)
	want := collect(t, p, testBlock, sampleOutput().ProtoReflect())

	for _, f := range factories() {
		t.Run(f.name, func(t *testing.T) {
			assert.Equal(t, want, collect(t, p, testBlock, f.make(t, sampleOutput())))
		})
	}
}

func TestWalk_RowContext(t *testing.T) {
	p := newRelationsProducer(t)

	seen := 0
	for row, err := range p.Rows(testBlock, sampleOutput().ProtoReflect()) {
		require.NoError(t, err)
		seen++

		assert.Equal(t, OpCreate, row.Op)
		assert.Equal(t, testBlock, *row.Block, "every row points at the block of the call")
		assert.Len(t, row.Values, len(row.Table.Columns))

		if row.Table.Name == "order_items" {
			require.NotNil(t, row.Parent)
			assert.Equal(t, "orders", row.Parent.Table.Name)
			value, ok := row.Parent.Value(row.Parent.Table.Columns[0].ID)
			require.True(t, ok)
			assert.Equal(t, "o1", value.String(), "the parent row stays valid for the whole subtree")
			assert.Equal(t, 0, row.Key().Len(), "order_items has no primary key")
		}
		if row.Table.Name == "orders" {
			assert.Equal(t, 1, row.Key().Len())
			key, err := row.Key().AppendValue(nil, 0, nil)
			require.NoError(t, err)
			assert.Equal(t, "o1", string(key))
		}
	}
	assert.Equal(t, 7, seen)
}

func TestWalk_DetachSurvivesReuse(t *testing.T) {
	p := newRelationsProducer(t)

	var kept []*Row
	for row, err := range p.Rows(testBlock, sampleOutput().ProtoReflect()) {
		require.NoError(t, err)
		kept = append(kept, row.Detach())
	}

	require.Len(t, kept, 7)
	assert.Equal(t, "customers", kept[0].Table.Name)
	assert.Equal(t, "c1", kept[0].Values[0].String())
	assert.Equal(t, "o1", kept[2].Parent.Values[0].String(), "the detached parent chain is a copy")
	assert.Equal(t, "i2", kept[3].Values[kept[3].Table.Column("item_id").ID].String())
	assert.Equal(t, testBlock, *kept[3].Block)
}

// TestWalk_Retry: calling Rows again gives the same rows in the same order with the same positions.
func TestWalk_Retry(t *testing.T) {
	p := newRelationsProducer(t)
	msg := sampleOutput().ProtoReflect()

	first := collect(t, p, testBlock, msg)
	second := collect(t, p, testBlock, msg)
	assert.Equal(t, first, second)

	// And a consumer can resume in the middle of the block: skip what it delivered, by SeqBlock.
	var resumed []snapshot
	for _, row := range second {
		if row.SeqBlock >= 4 {
			resumed = append(resumed, row)
		}
	}
	assert.Equal(t, first[4:], resumed)
}

func TestWalk_EarlyBreak(t *testing.T) {
	p := newRelationsProducer(t)

	count := 0
	for _, err := range p.Rows(testBlock, sampleOutput().ProtoReflect()) {
		require.NoError(t, err)
		count++
		if count == 3 {
			break
		}
	}
	assert.Equal(t, 3, count)

	assert.Len(t, collect(t, p, testBlock, sampleOutput().ProtoReflect()), 7, "a stopped walk leaves nothing behind")
}

func TestWalk_Errors(t *testing.T) {
	p := newRelationsProducer(t)

	for _, err := range p.Rows(testBlock, nil) {
		require.Error(t, err)
	}

	for row, err := range p.Rows(testBlock, (&relations.Customer{CustomerId: "x"}).ProtoReflect()) {
		assert.Nil(t, row)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "was not built from the descriptor instance")
	}
}

func TestWalk_EmptyOutput(t *testing.T) {
	p := newRelationsProducer(t)
	assert.Empty(t, collect(t, p, testBlock, (&relations.Output{}).ProtoReflect()))
}

func TestWalk_ConcurrentCalls(t *testing.T) {
	p := newRelationsProducer(t)
	want := collect(t, p, testBlock, sampleOutput().ProtoReflect())

	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for range 50 {
				var got []snapshot
				for row, err := range p.Rows(testBlock, sampleOutput().ProtoReflect()) {
					if err != nil {
						assert.NoError(t, err)
						return
					}
					got = append(got, snapshot{Table: row.Table.Name, Index: row.Index, SeqBlock: row.SeqBlock})
				}
				if !assert.Len(t, got, len(want)) {
					return
				}
				for i := range got {
					assert.Equal(t, want[i].Table, got[i].Table)
					assert.Equal(t, want[i].Index, got[i].Index)
				}
			}
		})
	}
	wg.Wait()
}

func TestWalk_Check(t *testing.T) {
	bad := sampleOutput()
	bad.Entities[2].GetTypesTest().Str_2Uint256 = "-5"

	// Off by default: the walk does not look at the text.
	unchecked := newRelationsProducer(t)
	for _, err := range unchecked.Rows(testBlock, bad.ProtoReflect()) {
		require.NoError(t, err)
	}

	checked := newRelationsProducer(t, WithCheck())
	var got error
	count := 0
	for _, err := range checked.Rows(testBlock, bad.ProtoReflect()) {
		count++
		if err != nil {
			got = err
		}
	}
	require.Error(t, got)
	assert.True(t, errors.Is(got, ErrInvalidValue))

	var validation *ValidationError
	require.ErrorAs(t, got, &validation)
	assert.Equal(t, "types_tests", validation.Table)
	assert.Equal(t, "str_2_uint256", validation.Column)
	assert.EqualValues(t, 100, validation.Block.Number)
	assert.Equal(t, "-5", validation.Value)
	assert.Equal(t, 6, count, "5 rows before the failing one, then the error ends the stream")

	// A valid block passes.
	for _, err := range checked.Rows(testBlock, sampleOutput().ProtoReflect()) {
		require.NoError(t, err)
	}
}

// TestWalk_DescriptorFromAnotherRegistry: a message of the right type but built from a copy of the descriptor would make
// protobuf-go panic on the cached field descriptors, so it is refused with an error instead.
func TestWalk_DescriptorFromAnotherRegistry(t *testing.T) {
	p := newRelationsProducer(t)

	fdp := protodesc.ToFileDescriptorProto(relationsDescriptor().ParentFile())
	copied, err := protodesc.NewFile(fdp, protoregistry.GlobalFiles)
	require.NoError(t, err)

	other := dynamicpb.NewMessage(copied.Messages().ByName("Output"))
	require.Equal(t, relationsDescriptor().FullName(), other.Descriptor().FullName())

	count := 0
	for row, err := range p.Rows(testBlock, other) {
		count++
		assert.Nil(t, row)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "another registry")
	}
	assert.Equal(t, 1, count)
}
