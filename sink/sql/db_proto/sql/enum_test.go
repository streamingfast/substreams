package sql

import (
	"testing"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/dynamicpb"
)

// TestScalarFieldValueUnwrapsEnums covers the crash any from-proto substreams carrying an
// enum field used to hit.
//
// protoreflect hands an enum out as a protoreflect.EnumNumber, a named int32 type that
// no dialect's type switch matched: PostgreSQL panicked with "unsupported type:
// protoreflect.EnumNumber" and ClickHouse with a failed value.(int32) assertion.
func TestScalarFieldValueUnwrapsEnums(t *testing.T) {
	message := &pbsubstreams.Module_KindStore{
		UpdatePolicy: pbsubstreams.Module_KindStore_UPDATE_POLICY_SET_IF_NOT_EXISTS,
	}

	encoded, err := proto.Marshal(message)
	require.NoError(t, err)

	descriptor := message.ProtoReflect().Descriptor()
	dynamic := dynamicpb.NewMessage(descriptor)
	require.NoError(t, proto.Unmarshal(encoded, dynamic))

	field := descriptor.Fields().ByName("update_policy")
	require.NotNil(t, field)

	value := scalarFieldValue(field, dynamic.Get(field))

	enum, ok := value.(EnumValue)
	require.True(t, ok, "expected an EnumValue, got %T", value)
	assert.Equal(t, int32(2), enum.Number)
	assert.Equal(t, "UPDATE_POLICY_SET_IF_NOT_EXISTS", enum.Name)
	assert.Equal(t, "UPDATE_POLICY_SET_IF_NOT_EXISTS", enum.String())
}

// TestScalarFieldValueLeavesOtherKindsAlone guards against the unwrapping widening to
// kinds the inserters already handle.
func TestScalarFieldValueLeavesOtherKindsAlone(t *testing.T) {
	message := &pbsubstreams.Module{Name: "map_something", InitialBlock: 42}

	descriptor := message.ProtoReflect().Descriptor()
	dynamic := dynamicpb.NewMessage(descriptor)
	encoded, err := proto.Marshal(message)
	require.NoError(t, err)
	require.NoError(t, proto.Unmarshal(encoded, dynamic))

	name := descriptor.Fields().ByName("name")
	assert.Equal(t, "map_something", scalarFieldValue(name, dynamic.Get(name)))

	initialBlock := descriptor.Fields().ByName("initial_block")
	assert.Equal(t, uint64(42), scalarFieldValue(initialBlock, dynamic.Get(initialBlock)))
}

// TestEnumValueFallsBackToTheNumber covers a value the descriptor does not know, which a
// proto3 reader has to tolerate rather than reject.
func TestEnumValueFallsBackToTheNumber(t *testing.T) {
	assert.Equal(t, "7", EnumValue{Number: 7}.String())
	assert.Equal(t, "KNOWN", EnumValue{Number: 7, Name: "KNOWN"}.String())
}
