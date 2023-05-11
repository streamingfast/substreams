package wasm

import (
	"testing"

	"github.com/stretchr/testify/assert"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

func Test_IsValidSetStore(t *testing.T) {
	tests := []struct {
		name     string
		instance *Call
		expect   bool
	}{
		{
			name:     "golden path",
			instance: &Call{updatePolicy: pbsubstreams.Module_KindStore_UPDATE_POLICY_SET, valueType: "int64"},
			expect:   true,
		},
		{
			name:     "wrong policy",
			instance: &Call{updatePolicy: pbsubstreams.Module_KindStore_UPDATE_POLICY_SET_IF_NOT_EXISTS, valueType: "int64"},
			expect:   false,
		},
		{
			name:     "different value",
			instance: &Call{updatePolicy: pbsubstreams.Module_KindStore_UPDATE_POLICY_SET, valueType: "bigdecimal"},
			expect:   true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			expectPanic(t, test.expect, test.instance.validateSetStore)
		})
	}
}

func expectPanic(t *testing.T, shouldPanic bool, f func(key string)) {
	if shouldPanic {
		assert.NotPanics(t, func() { f("key") })
	} else {
		assert.Panics(t, func() { f("key") })
	}
}

func Test_IsValidSetIfNotExists(t *testing.T) {
	tests := []struct {
		name     string
		instance *Call
		expect   bool
	}{
		{
			name:     "golden path",
			instance: &Call{updatePolicy: pbsubstreams.Module_KindStore_UPDATE_POLICY_SET_IF_NOT_EXISTS, valueType: "int64"},
			expect:   true,
		},
		{
			name:     "wrong policy",
			instance: &Call{updatePolicy: pbsubstreams.Module_KindStore_UPDATE_POLICY_SET, valueType: "int64"},
			expect:   false,
		},
		{
			name:     "different value",
			instance: &Call{updatePolicy: pbsubstreams.Module_KindStore_UPDATE_POLICY_SET_IF_NOT_EXISTS, valueType: "bigdecimal"},
			expect:   true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			expectPanic(t, test.expect, test.instance.validateSetIfNotExists)
		})
	}
}
func Test_IsValidAppendStore(t *testing.T) {
	tests := []struct {
		name     string
		instance *Call
		expect   bool
	}{
		{
			name:     "golden path",
			instance: &Call{updatePolicy: pbsubstreams.Module_KindStore_UPDATE_POLICY_APPEND, valueType: "string"},
			expect:   true,
		},
		{
			name:     "wrong policy",
			instance: &Call{updatePolicy: pbsubstreams.Module_KindStore_UPDATE_POLICY_SET, valueType: "int64"},
			expect:   false,
		},
		{
			name:     "different value",
			instance: &Call{updatePolicy: pbsubstreams.Module_KindStore_UPDATE_POLICY_APPEND, valueType: "bigdecimal"},
			expect:   true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			expectPanic(t, test.expect, test.instance.validateAppend)
		})
	}
}
func Test_IsValidAddBigIntStore(t *testing.T) {
	tests := []struct {
		name     string
		instance *Call
		expect   bool
	}{
		{
			name:     "golden path",
			instance: &Call{updatePolicy: pbsubstreams.Module_KindStore_UPDATE_POLICY_ADD, valueType: "bigint"},
			expect:   true,
		},
		{
			name:     "wrong type",
			instance: &Call{updatePolicy: pbsubstreams.Module_KindStore_UPDATE_POLICY_ADD, valueType: "int64"},
		},
		{
			name:     "wrong policy",
			instance: &Call{updatePolicy: pbsubstreams.Module_KindStore_UPDATE_POLICY_MAX, valueType: "bigint"},
		},
		{
			name:     "wrong policy, type",
			instance: &Call{updatePolicy: pbsubstreams.Module_KindStore_UPDATE_POLICY_MAX, valueType: "int64"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			expectPanic(t, test.expect, test.instance.validateAddBigInt)
		})
	}
}
func Test_IsValidAddBigDecimalStore(t *testing.T) {
	tests := []struct {
		name     string
		instance *Call
		expect   bool
	}{
		{
			name:     "golden path",
			instance: &Call{updatePolicy: pbsubstreams.Module_KindStore_UPDATE_POLICY_ADD, valueType: "bigdecimal"},
			expect:   true,
		},
		{
			name:     "wrong type",
			instance: &Call{updatePolicy: pbsubstreams.Module_KindStore_UPDATE_POLICY_ADD, valueType: "float64"},
		},
		{
			name:     "wrong policy",
			instance: &Call{updatePolicy: pbsubstreams.Module_KindStore_UPDATE_POLICY_MAX, valueType: "bigdecimal"},
		},
		{
			name:     "wrong policy, type",
			instance: &Call{updatePolicy: pbsubstreams.Module_KindStore_UPDATE_POLICY_MAX, valueType: "int64"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			expectPanic(t, test.expect, test.instance.validateAddBigDecimal)
		})
	}
}
func Test_IsValidAddInt64Store(t *testing.T) {
	tests := []struct {
		name     string
		instance *Call
		expect   bool
	}{
		{
			name:     "golden path",
			instance: &Call{updatePolicy: pbsubstreams.Module_KindStore_UPDATE_POLICY_ADD, valueType: "int64"},
			expect:   true,
		},
		{
			name:     "wrong type",
			instance: &Call{updatePolicy: pbsubstreams.Module_KindStore_UPDATE_POLICY_ADD, valueType: "float64"},
		},
		{
			name:     "wrong policy",
			instance: &Call{updatePolicy: pbsubstreams.Module_KindStore_UPDATE_POLICY_MAX, valueType: "int64"},
		},
		{
			name:     "wrong policy, type",
			instance: &Call{updatePolicy: pbsubstreams.Module_KindStore_UPDATE_POLICY_MAX, valueType: "bigint"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			expectPanic(t, test.expect, test.instance.validateAddInt64)
		})
	}
}
func Test_IsValidAddFloat64Store(t *testing.T) {
	tests := []struct {
		name     string
		instance *Call
		expect   bool
	}{
		{
			name:     "golden path",
			instance: &Call{updatePolicy: pbsubstreams.Module_KindStore_UPDATE_POLICY_ADD, valueType: "float64"},
			expect:   true,
		},
		{
			name:     "wrong type",
			instance: &Call{updatePolicy: pbsubstreams.Module_KindStore_UPDATE_POLICY_ADD, valueType: "int64"},
		},
		{
			name:     "wrong policy",
			instance: &Call{updatePolicy: pbsubstreams.Module_KindStore_UPDATE_POLICY_MAX, valueType: "float64"},
		},
		{
			name:     "wrong policy, type",
			instance: &Call{updatePolicy: pbsubstreams.Module_KindStore_UPDATE_POLICY_MAX, valueType: "bigint"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			expectPanic(t, test.expect, test.instance.validateAddFloat64)
		})
	}
}
func Test_IsValidSetMintInt64Store(t *testing.T) {
	tests := []struct {
		name     string
		instance *Call
		expect   bool
	}{
		{
			name:     "golden path",
			instance: &Call{updatePolicy: pbsubstreams.Module_KindStore_UPDATE_POLICY_MIN, valueType: "int64"},
			expect:   true,
		},
		{
			name:     "wrong type",
			instance: &Call{updatePolicy: pbsubstreams.Module_KindStore_UPDATE_POLICY_MIN, valueType: "float64"},
		},
		{
			name:     "wrong policy",
			instance: &Call{updatePolicy: pbsubstreams.Module_KindStore_UPDATE_POLICY_MAX, valueType: "int64"},
		},
		{
			name:     "wrong policy, type",
			instance: &Call{updatePolicy: pbsubstreams.Module_KindStore_UPDATE_POLICY_MAX, valueType: "float64"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			expectPanic(t, test.expect, test.instance.validateSetMinInt64)
		})
	}
}
func Test_IsValidSetMintBigInt64Store(t *testing.T) {
	tests := []struct {
		name     string
		instance *Call
		expect   bool
	}{
		{
			name:     "golden path",
			instance: &Call{updatePolicy: pbsubstreams.Module_KindStore_UPDATE_POLICY_MIN, valueType: "bigint"},
			expect:   true,
		},
		{
			name:     "wrong type",
			instance: &Call{updatePolicy: pbsubstreams.Module_KindStore_UPDATE_POLICY_MIN, valueType: "int64"},
		},
		{
			name:     "wrong policy",
			instance: &Call{updatePolicy: pbsubstreams.Module_KindStore_UPDATE_POLICY_MAX, valueType: "bigint"},
		},
		{
			name:     "wrong policy, type",
			instance: &Call{updatePolicy: pbsubstreams.Module_KindStore_UPDATE_POLICY_MAX, valueType: "bigdecimal"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			expectPanic(t, test.expect, test.instance.validateSetMinBigInt)
		})
	}
}
func Test_IsValidSetMintFloat64Store(t *testing.T) {
	tests := []struct {
		name     string
		instance *Call
		expect   bool
	}{
		{
			name:     "golden path",
			instance: &Call{updatePolicy: pbsubstreams.Module_KindStore_UPDATE_POLICY_MIN, valueType: "float64"},
			expect:   true,
		},
		{
			name:     "wrong type",
			instance: &Call{updatePolicy: pbsubstreams.Module_KindStore_UPDATE_POLICY_MIN, valueType: "bigdecimal"},
		},
		{
			name:     "wrong policy",
			instance: &Call{updatePolicy: pbsubstreams.Module_KindStore_UPDATE_POLICY_MAX, valueType: "float64"},
		},
		{
			name:     "wrong policy, type",
			instance: &Call{updatePolicy: pbsubstreams.Module_KindStore_UPDATE_POLICY_MAX, valueType: "bigint"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			expectPanic(t, test.expect, test.instance.validateSetMinFloat64)
		})
	}
}
func Test_IsValidSetMinBigDecimalStore(t *testing.T) {
	tests := []struct {
		name     string
		instance *Call
		expect   bool
	}{
		{
			name:     "golden path",
			instance: &Call{updatePolicy: pbsubstreams.Module_KindStore_UPDATE_POLICY_MIN, valueType: "bigdecimal"},
			expect:   true,
		},
		{
			name:     "wrong type",
			instance: &Call{updatePolicy: pbsubstreams.Module_KindStore_UPDATE_POLICY_MIN, valueType: "bigint"},
		},
		{
			name:     "wrong policy",
			instance: &Call{updatePolicy: pbsubstreams.Module_KindStore_UPDATE_POLICY_MAX, valueType: "bigdecimal"},
		},
		{
			name:     "wrong policy, type",
			instance: &Call{updatePolicy: pbsubstreams.Module_KindStore_UPDATE_POLICY_MAX, valueType: "float64"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			expectPanic(t, test.expect, test.instance.validateSetMinBigDecimal)
		})
	}
}
func Test_IsValidSetMaxInt64Store(t *testing.T) {
	tests := []struct {
		name     string
		instance *Call
		expect   bool
	}{
		{
			name:     "golden path",
			instance: &Call{updatePolicy: pbsubstreams.Module_KindStore_UPDATE_POLICY_MAX, valueType: "int64"},
			expect:   true,
		},
		{
			name:     "wrong type",
			instance: &Call{updatePolicy: pbsubstreams.Module_KindStore_UPDATE_POLICY_MAX, valueType: "bigint"},
		},
		{
			name:     "wrong policy",
			instance: &Call{updatePolicy: pbsubstreams.Module_KindStore_UPDATE_POLICY_MIN, valueType: "int64"},
		},
		{
			name:     "wrong policy, type",
			instance: &Call{updatePolicy: pbsubstreams.Module_KindStore_UPDATE_POLICY_ADD, valueType: "float64"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			expectPanic(t, test.expect, test.instance.validateSetMaxInt64)
		})
	}
}
func Test_IsValidSetMaxBigIntStore(t *testing.T) {
	tests := []struct {
		name     string
		instance *Call
		expect   bool
	}{
		{
			name:     "golden path",
			instance: &Call{updatePolicy: pbsubstreams.Module_KindStore_UPDATE_POLICY_MAX, valueType: "bigint"},
			expect:   true,
		},
		{
			name:     "wrong type",
			instance: &Call{updatePolicy: pbsubstreams.Module_KindStore_UPDATE_POLICY_MAX, valueType: "int64"},
		},
		{
			name:     "wrong policy",
			instance: &Call{updatePolicy: pbsubstreams.Module_KindStore_UPDATE_POLICY_MIN, valueType: "bigint"},
		},
		{
			name:     "wrong policy, type",
			instance: &Call{updatePolicy: pbsubstreams.Module_KindStore_UPDATE_POLICY_ADD, valueType: "float64"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			expectPanic(t, test.expect, test.instance.validateSetMaxBigInt)
		})
	}
}
func Test_IsValidSetMaxFloat64Store(t *testing.T) {
	tests := []struct {
		name     string
		instance *Call
		expect   bool
	}{
		{
			name:     "golden path",
			instance: &Call{updatePolicy: pbsubstreams.Module_KindStore_UPDATE_POLICY_MAX, valueType: "float64"},
			expect:   true,
		},
		{
			name:     "wrong type",
			instance: &Call{updatePolicy: pbsubstreams.Module_KindStore_UPDATE_POLICY_MAX, valueType: "bigdecimal"},
		},
		{
			name:     "wrong policy",
			instance: &Call{updatePolicy: pbsubstreams.Module_KindStore_UPDATE_POLICY_MIN, valueType: "int64"},
		},
		{
			name:     "wrong policy, type",
			instance: &Call{updatePolicy: pbsubstreams.Module_KindStore_UPDATE_POLICY_ADD, valueType: "bigdecimal"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			expectPanic(t, test.expect, test.instance.validateSetMaxFloat64)
		})
	}
}
func Test_IsValidSetMaxBigDecimalStore(t *testing.T) {
	tests := []struct {
		name     string
		instance *Call
		expect   bool
	}{
		{
			name:     "golden path",
			instance: &Call{updatePolicy: pbsubstreams.Module_KindStore_UPDATE_POLICY_MAX, valueType: "bigdecimal"},
			expect:   true,
		},
		{
			name:     "wrong type",
			instance: &Call{updatePolicy: pbsubstreams.Module_KindStore_UPDATE_POLICY_MAX, valueType: "float64"},
		},
		{
			name:     "wrong policy",
			instance: &Call{updatePolicy: pbsubstreams.Module_KindStore_UPDATE_POLICY_MIN, valueType: "int64"},
		},
		{
			name:     "wrong policy, type",
			instance: &Call{updatePolicy: pbsubstreams.Module_KindStore_UPDATE_POLICY_ADD, valueType: "float64"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			expectPanic(t, test.expect, test.instance.validateSetMaxBigDecimal)
		})
	}
}
