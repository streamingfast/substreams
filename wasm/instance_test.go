package wasm

import (
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/stretchr/testify/assert"
	"testing"
)

func Test_IsValidSetStore(t *testing.T) {
	tests := []struct {
		name     string
		instance *Instance
		expect   bool
	}{
		{
			name:     "golden path",
			instance: &Instance{updatePolicy: pbsubstreams.Module_KindStore_UPDATE_POLICY_SET, valueType: "int64"},
			expect:   true,
		},
		{
			name:     "wrong policy",
			instance: &Instance{updatePolicy: pbsubstreams.Module_KindStore_UPDATE_POLICY_SET_IF_NOT_EXISTS, valueType: "int64"},
			expect:   false,
		},
		{
			name:     "different value",
			instance: &Instance{updatePolicy: pbsubstreams.Module_KindStore_UPDATE_POLICY_SET, valueType: "bigdecimal"},
			expect:   true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.expect, test.instance.IsValidSetStore())
		})
	}
}
func Test_IsValidSetIfNotExists(t *testing.T) {
	tests := []struct {
		name     string
		instance *Instance
		expect   bool
	}{
		{
			name:     "golden path",
			instance: &Instance{updatePolicy: pbsubstreams.Module_KindStore_UPDATE_POLICY_SET_IF_NOT_EXISTS, valueType: "int64"},
			expect:   true,
		},
		{
			name:     "wrong policy",
			instance: &Instance{updatePolicy: pbsubstreams.Module_KindStore_UPDATE_POLICY_SET, valueType: "int64"},
			expect:   false,
		},
		{
			name:     "different value",
			instance: &Instance{updatePolicy: pbsubstreams.Module_KindStore_UPDATE_POLICY_SET_IF_NOT_EXISTS, valueType: "bigdecimal"},
			expect:   true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.expect, test.instance.IsValidSetIfNotExists())
		})
	}
}
func Test_IsValidAppendStore(t *testing.T) {
	tests := []struct {
		name     string
		instance *Instance
		expect   bool
	}{
		{
			name:     "golden path",
			instance: &Instance{updatePolicy: pbsubstreams.Module_KindStore_UPDATE_POLICY_APPEND, valueType: "string"},
			expect:   true,
		},
		{
			name:     "wrong policy",
			instance: &Instance{updatePolicy: pbsubstreams.Module_KindStore_UPDATE_POLICY_SET, valueType: "int64"},
			expect:   false,
		},
		{
			name:     "different value",
			instance: &Instance{updatePolicy: pbsubstreams.Module_KindStore_UPDATE_POLICY_APPEND, valueType: "bigdecimal"},
			expect:   true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.expect, test.instance.IsValidAppendStore())
		})
	}
}
func Test_IsValidAddBigIntStore(t *testing.T) {
	tests := []struct {
		name     string
		instance *Instance
		expect   bool
	}{
		{
			name:     "golden path",
			instance: &Instance{updatePolicy: pbsubstreams.Module_KindStore_UPDATE_POLICY_ADD, valueType: "bigint"},
			expect:   true,
		},
		{
			name:     "wrong type",
			instance: &Instance{updatePolicy: pbsubstreams.Module_KindStore_UPDATE_POLICY_ADD, valueType: "int64"},
		},
		{
			name:     "wrong policy",
			instance: &Instance{updatePolicy: pbsubstreams.Module_KindStore_UPDATE_POLICY_MAX, valueType: "bigint"},
		},
		{
			name:     "wrong policy, type",
			instance: &Instance{updatePolicy: pbsubstreams.Module_KindStore_UPDATE_POLICY_MAX, valueType: "int64"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.expect, test.instance.IsValidAddBigIntStore())
		})
	}
}
func Test_IsValidAddBigDecimalStore(t *testing.T) {
	tests := []struct {
		name     string
		instance *Instance
		expect   bool
	}{
		{
			name:     "golden path",
			instance: &Instance{updatePolicy: pbsubstreams.Module_KindStore_UPDATE_POLICY_ADD, valueType: "bigdecimal"},
			expect:   true,
		},
		{
			name:     "wrong type",
			instance: &Instance{updatePolicy: pbsubstreams.Module_KindStore_UPDATE_POLICY_ADD, valueType: "float64"},
		},
		{
			name:     "wrong policy",
			instance: &Instance{updatePolicy: pbsubstreams.Module_KindStore_UPDATE_POLICY_MAX, valueType: "bigdecimal"},
		},
		{
			name:     "wrong policy, type",
			instance: &Instance{updatePolicy: pbsubstreams.Module_KindStore_UPDATE_POLICY_MAX, valueType: "int64"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.expect, test.instance.IsValidAddBigDecimalStore())
		})
	}
}
func Test_IsValidAddInt64Store(t *testing.T) {
	tests := []struct {
		name     string
		instance *Instance
		expect   bool
	}{
		{
			name:     "golden path",
			instance: &Instance{updatePolicy: pbsubstreams.Module_KindStore_UPDATE_POLICY_ADD, valueType: "int64"},
			expect:   true,
		},
		{
			name:     "wrong type",
			instance: &Instance{updatePolicy: pbsubstreams.Module_KindStore_UPDATE_POLICY_ADD, valueType: "float64"},
		},
		{
			name:     "wrong policy",
			instance: &Instance{updatePolicy: pbsubstreams.Module_KindStore_UPDATE_POLICY_MAX, valueType: "int64"},
		},
		{
			name:     "wrong policy, type",
			instance: &Instance{updatePolicy: pbsubstreams.Module_KindStore_UPDATE_POLICY_MAX, valueType: "bigint"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.expect, test.instance.IsValidAddInt64Store())
		})
	}
}
func Test_IsValidAddFloat64Store(t *testing.T) {
	tests := []struct {
		name     string
		instance *Instance
		expect   bool
	}{
		{
			name:     "golden path",
			instance: &Instance{updatePolicy: pbsubstreams.Module_KindStore_UPDATE_POLICY_ADD, valueType: "float64"},
			expect:   true,
		},
		{
			name:     "wrong type",
			instance: &Instance{updatePolicy: pbsubstreams.Module_KindStore_UPDATE_POLICY_ADD, valueType: "int64"},
		},
		{
			name:     "wrong policy",
			instance: &Instance{updatePolicy: pbsubstreams.Module_KindStore_UPDATE_POLICY_MAX, valueType: "float64"},
		},
		{
			name:     "wrong policy, type",
			instance: &Instance{updatePolicy: pbsubstreams.Module_KindStore_UPDATE_POLICY_MAX, valueType: "bigint"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.expect, test.instance.IsValidAddFloat64Store())
		})
	}
}
func Test_IsValidSetMintInt64Store(t *testing.T) {
	tests := []struct {
		name     string
		instance *Instance
		expect   bool
	}{
		{
			name:     "golden path",
			instance: &Instance{updatePolicy: pbsubstreams.Module_KindStore_UPDATE_POLICY_MIN, valueType: "int64"},
			expect:   true,
		},
		{
			name:     "wrong type",
			instance: &Instance{updatePolicy: pbsubstreams.Module_KindStore_UPDATE_POLICY_MIN, valueType: "float64"},
		},
		{
			name:     "wrong policy",
			instance: &Instance{updatePolicy: pbsubstreams.Module_KindStore_UPDATE_POLICY_MAX, valueType: "int64"},
		},
		{
			name:     "wrong policy, type",
			instance: &Instance{updatePolicy: pbsubstreams.Module_KindStore_UPDATE_POLICY_MAX, valueType: "float64"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.expect, test.instance.IsValidSetMinInt64Store())
		})
	}
}
func Test_IsValidSetMintBigInt64Store(t *testing.T) {
	tests := []struct {
		name     string
		instance *Instance
		expect   bool
	}{
		{
			name:     "golden path",
			instance: &Instance{updatePolicy: pbsubstreams.Module_KindStore_UPDATE_POLICY_MIN, valueType: "bigint"},
			expect:   true,
		},
		{
			name:     "wrong type",
			instance: &Instance{updatePolicy: pbsubstreams.Module_KindStore_UPDATE_POLICY_MIN, valueType: "int64"},
		},
		{
			name:     "wrong policy",
			instance: &Instance{updatePolicy: pbsubstreams.Module_KindStore_UPDATE_POLICY_MAX, valueType: "bigint"},
		},
		{
			name:     "wrong policy, type",
			instance: &Instance{updatePolicy: pbsubstreams.Module_KindStore_UPDATE_POLICY_MAX, valueType: "bigdecimal"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.expect, test.instance.IsValidSetMinBigIntStore())
		})
	}
}
func Test_IsValidSetMintFloat64Store(t *testing.T) {
	tests := []struct {
		name     string
		instance *Instance
		expect   bool
	}{
		{
			name:     "golden path",
			instance: &Instance{updatePolicy: pbsubstreams.Module_KindStore_UPDATE_POLICY_MIN, valueType: "float64"},
			expect:   true,
		},
		{
			name:     "wrong type",
			instance: &Instance{updatePolicy: pbsubstreams.Module_KindStore_UPDATE_POLICY_MIN, valueType: "bigdecimal"},
		},
		{
			name:     "wrong policy",
			instance: &Instance{updatePolicy: pbsubstreams.Module_KindStore_UPDATE_POLICY_MAX, valueType: "float64"},
		},
		{
			name:     "wrong policy, type",
			instance: &Instance{updatePolicy: pbsubstreams.Module_KindStore_UPDATE_POLICY_MAX, valueType: "bigint"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.expect, test.instance.IsValidSetMinFloat64Store())
		})
	}
}
func Test_IsValidSetMinBigDecimalStore(t *testing.T) {
	tests := []struct {
		name     string
		instance *Instance
		expect   bool
	}{
		{
			name:     "golden path",
			instance: &Instance{updatePolicy: pbsubstreams.Module_KindStore_UPDATE_POLICY_MIN, valueType: "bigdecimal"},
			expect:   true,
		},
		{
			name:     "wrong type",
			instance: &Instance{updatePolicy: pbsubstreams.Module_KindStore_UPDATE_POLICY_MIN, valueType: "bigint"},
		},
		{
			name:     "wrong policy",
			instance: &Instance{updatePolicy: pbsubstreams.Module_KindStore_UPDATE_POLICY_MAX, valueType: "bigdecimal"},
		},
		{
			name:     "wrong policy, type",
			instance: &Instance{updatePolicy: pbsubstreams.Module_KindStore_UPDATE_POLICY_MAX, valueType: "float64"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.expect, test.instance.IsValidSetMinBigDecimalStore())
		})
	}
}
func Test_IsValidSetMaxInt64Store(t *testing.T) {
	tests := []struct {
		name     string
		instance *Instance
		expect   bool
	}{
		{
			name:     "golden path",
			instance: &Instance{updatePolicy: pbsubstreams.Module_KindStore_UPDATE_POLICY_MAX, valueType: "int64"},
			expect:   true,
		},
		{
			name:     "wrong type",
			instance: &Instance{updatePolicy: pbsubstreams.Module_KindStore_UPDATE_POLICY_MAX, valueType: "bigint"},
		},
		{
			name:     "wrong policy",
			instance: &Instance{updatePolicy: pbsubstreams.Module_KindStore_UPDATE_POLICY_MIN, valueType: "int64"},
		},
		{
			name:     "wrong policy, type",
			instance: &Instance{updatePolicy: pbsubstreams.Module_KindStore_UPDATE_POLICY_ADD, valueType: "float64"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.expect, test.instance.IsValidSetMaxInt64Store())
		})
	}
}
func Test_IsValidSetMaxBigIntStore(t *testing.T) {
	tests := []struct {
		name     string
		instance *Instance
		expect   bool
	}{
		{
			name:     "golden path",
			instance: &Instance{updatePolicy: pbsubstreams.Module_KindStore_UPDATE_POLICY_MAX, valueType: "bigint"},
			expect:   true,
		},
		{
			name:     "wrong type",
			instance: &Instance{updatePolicy: pbsubstreams.Module_KindStore_UPDATE_POLICY_MAX, valueType: "int64"},
		},
		{
			name:     "wrong policy",
			instance: &Instance{updatePolicy: pbsubstreams.Module_KindStore_UPDATE_POLICY_MIN, valueType: "bigint"},
		},
		{
			name:     "wrong policy, type",
			instance: &Instance{updatePolicy: pbsubstreams.Module_KindStore_UPDATE_POLICY_ADD, valueType: "float64"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.expect, test.instance.IsValidSetMaxBigIntStore())
		})
	}
}
func Test_IsValidSetMaxFloat64Store(t *testing.T) {
	tests := []struct {
		name     string
		instance *Instance
		expect   bool
	}{
		{
			name:     "golden path",
			instance: &Instance{updatePolicy: pbsubstreams.Module_KindStore_UPDATE_POLICY_MAX, valueType: "float64"},
			expect:   true,
		},
		{
			name:     "wrong type",
			instance: &Instance{updatePolicy: pbsubstreams.Module_KindStore_UPDATE_POLICY_MAX, valueType: "bigdecimal"},
		},
		{
			name:     "wrong policy",
			instance: &Instance{updatePolicy: pbsubstreams.Module_KindStore_UPDATE_POLICY_MIN, valueType: "int64"},
		},
		{
			name:     "wrong policy, type",
			instance: &Instance{updatePolicy: pbsubstreams.Module_KindStore_UPDATE_POLICY_ADD, valueType: "bigdecimal"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.expect, test.instance.IsValidSetMaxFloat64Store())
		})
	}
}
func Test_IsValidSetMaxBigDecimalStore(t *testing.T) {
	tests := []struct {
		name     string
		instance *Instance
		expect   bool
	}{
		{
			name:     "golden path",
			instance: &Instance{updatePolicy: pbsubstreams.Module_KindStore_UPDATE_POLICY_MAX, valueType: "bigdecimal"},
			expect:   true,
		},
		{
			name:     "wrong type",
			instance: &Instance{updatePolicy: pbsubstreams.Module_KindStore_UPDATE_POLICY_MAX, valueType: "float64"},
		},
		{
			name:     "wrong policy",
			instance: &Instance{updatePolicy: pbsubstreams.Module_KindStore_UPDATE_POLICY_MIN, valueType: "int64"},
		},
		{
			name:     "wrong policy, type",
			instance: &Instance{updatePolicy: pbsubstreams.Module_KindStore_UPDATE_POLICY_ADD, valueType: "float64"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.expect, test.instance.IsValidSetMaxBigDecimalStore())
		})
	}
}
