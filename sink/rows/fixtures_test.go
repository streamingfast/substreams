package rows

import (
	"testing"

	"buf.build/go/hyperpb"
	"github.com/streamingfast/protox"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"

	"github.com/streamingfast/substreams/sink/sql/tests/relations"
)

func relationsDescriptor() protoreflect.MessageDescriptor {
	return (&relations.Output{}).ProtoReflect().Descriptor()
}

func ptr[T any](v T) *T { return &v }

// sampleOutput has, in entity order: a customer, an order with two items and an extension, a types test and an item.
func sampleOutput() *relations.Output {
	return &relations.Output{Entities: []*relations.Entity{
		{Entity: &relations.Entity_Customer{Customer: &relations.Customer{CustomerId: "c1", Name: "Alice"}}},
		{Entity: &relations.Entity_Order{Order: &relations.Order{
			OrderId:       "o1",
			CustomerRefId: "c1",
			Items:         []*relations.OrderItem{{ItemId: "i1", Quantity: 2}, {ItemId: "i2", Quantity: 3}},
			Extension:     &relations.OrderExtension{Description: "gift"},
		}}},
		{Entity: &relations.Entity_TypesTest{TypesTest: &relations.TypesTest{
			Id:                    7,
			StringField:           "hello",
			BytesField:            []byte{1, 2},
			OptionalStringSet:     ptr("set"),
			OptionalInt32FieldSet: ptr(int32(5)),
			RepeatedInt32Field:    []int32{1, 2, 3},
			Str_2Uint256:          "115792089237316195423570985008687907853269984665640564039457584007913129639935",
			Str_2Decimal128:       "12.3456",
			Level1:                &relations.NestedLevel1{Name: "n"},
		}}},
		{Entity: &relations.Entity_Item{Item: &relations.Item{ItemId: "i1", Name: "Widget", Price: 9.5}}},
	}}
}

type messageFactory struct {
	name string
	make func(t *testing.T, m proto.Message) protoreflect.Message
}

// factories give the same wire data as a dynamicpb message and as a hyperpb one, the two kinds db_proto can run on.
func factories() []messageFactory {
	return []messageFactory{
		{"generated", func(t *testing.T, m proto.Message) protoreflect.Message { return m.ProtoReflect() }},
		{"dynamicpb", func(t *testing.T, m proto.Message) protoreflect.Message {
			t.Helper()
			payload, err := proto.Marshal(m)
			require.NoError(t, err)
			dynamic := dynamicpb.NewMessage(m.ProtoReflect().Descriptor())
			require.NoError(t, proto.Unmarshal(payload, dynamic))
			return dynamic
		}},
		{"hyperpb", func(t *testing.T, m proto.Message) protoreflect.Message {
			t.Helper()
			payload, err := proto.Marshal(m)
			require.NoError(t, err)
			msg := new(hyperpb.Shared).NewMessage(hyperpb.CompileMessageDescriptor(m.ProtoReflect().Descriptor()))
			require.NoError(t, msg.Unmarshal(payload))
			return msg.ProtoReflect()
		}},
	}
}

// snapshot is a detached, comparable copy of a row.
type snapshot struct {
	Table    string
	Index    uint32
	SeqBlock uint32
	Parent   string
	Values   map[string]any
}

func collect(t *testing.T, p RowProducer, blk BlockRef, msg protoreflect.Message) []snapshot {
	t.Helper()

	var out []snapshot
	for row, err := range p.Rows(blk, msg) {
		require.NoError(t, err)

		snap := snapshot{Table: row.Table.Name, Index: row.Index, SeqBlock: row.SeqBlock, Values: map[string]any{}}
		if row.Parent != nil {
			snap.Parent = row.Parent.Table.Name
		}
		for i := range row.Table.Columns {
			column := &row.Table.Columns[i]
			if value, ok := row.Value(column.ID); ok {
				snap.Values[column.Name] = plain(column.Type, value)
			}
		}
		out = append(out, snap)
	}
	return out
}

// plain turns a value into a Go value comparable across message implementations.
func plain(t LogicalType, v protoreflect.Value) any {
	switch t.Kind {
	case KindString, KindBigint, KindDecimal:
		return v.String()
	case KindBool:
		return v.Bool()
	case KindInt32, KindInt64:
		return v.Int()
	case KindUint32, KindUint64:
		return v.Uint()
	case KindFloat32, KindFloat64:
		return v.Float()
	case KindBytes:
		return string(v.Bytes())
	case KindEnum:
		return int64(v.Enum())
	case KindTimestamp:
		return protox.DynamicAsTimestampTime(v.Message())
	case KindStruct:
		return "struct"
	case KindList:
		list := v.List()
		out := make([]any, list.Len())
		for i := range out {
			out[i] = plain(t.ElemType(), list.Get(i))
		}
		return out
	}
	return nil
}
