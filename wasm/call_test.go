package wasm

import (
	"testing"

	"github.com/streamingfast/dstore"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"

	"github.com/streamingfast/substreams/metrics"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/storage/store"
)

func Test_CallStoreOps(t *testing.T) {
	tests := []struct {
		name        string
		instance    *Call
		testFunc    func(*Call)
		expectPanic bool
	}{
		{"set golden path",
			newTestCall(pbsubstreams.Module_KindStore_UPDATE_POLICY_SET, "int64"),
			func(c *Call) {
				c.DoSet(0, "key", []byte("value"))
			},
			true,
		},
		{"set wrong policy",
			newTestCall(pbsubstreams.Module_KindStore_UPDATE_POLICY_SET_IF_NOT_EXISTS, "int64"),
			func(c *Call) {
				c.DoSet(0, "key", []byte("value"))
			},
			false,
		},
		{"set different value",
			newTestCall(pbsubstreams.Module_KindStore_UPDATE_POLICY_SET, "bigdecimal"),
			func(c *Call) {
				c.DoSet(0, "key", []byte("value"))
			},
			true,
		},
		{
			"set_if_not_exists golden path",
			newTestCall(pbsubstreams.Module_KindStore_UPDATE_POLICY_SET_IF_NOT_EXISTS, "int64"),
			func(c *Call) {
				c.DoSetIfNotExists(0, "key", []byte("value"))
			},
			true,
		},
		{
			"set_if_not_exists wrong policy",
			newTestCall(pbsubstreams.Module_KindStore_UPDATE_POLICY_SET, "int64"),
			func(c *Call) {
				c.DoSetIfNotExists(0, "key", []byte("value"))
			},
			false,
		},
		{
			"set_if_not_exists different value",
			newTestCall(pbsubstreams.Module_KindStore_UPDATE_POLICY_SET_IF_NOT_EXISTS, "bigdecimal"),
			func(c *Call) {
				c.DoSetIfNotExists(0, "key", []byte("value"))
			},
			true,
		},
		{
			"append golden path",
			newTestCall(pbsubstreams.Module_KindStore_UPDATE_POLICY_APPEND, "string"),
			func(c *Call) {
				c.DoAppend(0, "key", []byte("value"))
			},
			true,
		},
		{
			"append wrong policy",
			newTestCall(pbsubstreams.Module_KindStore_UPDATE_POLICY_SET, "int64"),
			func(c *Call) {
				c.DoAppend(0, "key", []byte("value"))
			},
			false,
		},
		{
			"append different value",
			newTestCall(pbsubstreams.Module_KindStore_UPDATE_POLICY_APPEND, "bigdecimal"),
			func(c *Call) {
				c.DoAppend(0, "key", []byte("value"))
			},
			true,
		},
		{
			"add_bigint golden path",
			newTestCall(pbsubstreams.Module_KindStore_UPDATE_POLICY_ADD, "bigint"),
			func(c *Call) {
				c.DoAddBigInt(0, "key", "1")
			},
			true,
		},
		{
			"add_bigint wrong type",
			newTestCall(pbsubstreams.Module_KindStore_UPDATE_POLICY_ADD, "int64"),
			func(c *Call) {
				c.DoAddBigInt(0, "key", "1")
			},
			false,
		},
		{
			"add_bigint wrong policy",
			newTestCall(pbsubstreams.Module_KindStore_UPDATE_POLICY_MAX, "bigint"),
			func(c *Call) {
				c.DoAddBigInt(0, "key", "1")
			},
			false,
		},
		{
			"add_bigint wrong policy, type",
			newTestCall(pbsubstreams.Module_KindStore_UPDATE_POLICY_MAX, "int64"),
			func(c *Call) {
				c.DoAddBigInt(0, "key", "1")
			},
			false,
		},
		{
			"add_bigdecimal golden path",
			newTestCall(pbsubstreams.Module_KindStore_UPDATE_POLICY_ADD, "bigdecimal"),
			func(c *Call) {
				c.DoAddBigDecimal(0, "key", "1.0")
			},
			true,
		},
		{
			"add_bigdecimal wrong type",
			newTestCall(pbsubstreams.Module_KindStore_UPDATE_POLICY_ADD, "float64"),
			func(c *Call) {
				c.DoAddBigDecimal(0, "key", "1.0")
			},
			false,
		},
		{
			"add_bigdecimal wrong policy",
			newTestCall(pbsubstreams.Module_KindStore_UPDATE_POLICY_MAX, "bigdecimal"),
			func(c *Call) {
				c.DoAddBigDecimal(0, "key", "1.0")
			},
			false,
		},
		{
			"add_bigdecimal wrong policy, type",
			newTestCall(pbsubstreams.Module_KindStore_UPDATE_POLICY_MAX, "int64"),
			func(c *Call) {
				c.DoAddBigDecimal(0, "key", "1.0")
			},
			false,
		},
		{
			"add_int64 golden path",
			newTestCall(pbsubstreams.Module_KindStore_UPDATE_POLICY_ADD, "int64"),
			func(c *Call) {
				c.DoAddInt64(0, "key", 1)
			},
			true,
		},
		{
			"add_int64 wrong type",
			newTestCall(pbsubstreams.Module_KindStore_UPDATE_POLICY_ADD, "float64"),
			func(c *Call) {
				c.DoAddInt64(0, "key", 1)
			},
			false,
		},
		{
			"add_int64 wrong policy",
			newTestCall(pbsubstreams.Module_KindStore_UPDATE_POLICY_MAX, "int64"),
			func(c *Call) {
				c.DoAddInt64(0, "key", 1)
			},
			false,
		},
		{
			"add_int64 wrong policy, type",
			newTestCall(pbsubstreams.Module_KindStore_UPDATE_POLICY_MAX, "bigint"),
			func(c *Call) {
				c.DoAddInt64(0, "key", 1)
			},
			false,
		},
		{
			"add_float64 golden path",
			newTestCall(pbsubstreams.Module_KindStore_UPDATE_POLICY_ADD, "float64"),
			func(c *Call) {
				c.DoAddFloat64(0, "key", 1.0)
			},
			true,
		},
		{
			"add_float64 wrong type",
			newTestCall(pbsubstreams.Module_KindStore_UPDATE_POLICY_ADD, "int64"),
			func(c *Call) {
				c.DoAddFloat64(0, "key", 1.0)
			},
			false,
		},
		{
			"add_float64 wrong policy",
			newTestCall(pbsubstreams.Module_KindStore_UPDATE_POLICY_MAX, "float64"),
			func(c *Call) {
				c.DoAddFloat64(0, "key", 1.0)
			},
			false,
		},
		{
			"add_float64 wrong policy, type",
			newTestCall(pbsubstreams.Module_KindStore_UPDATE_POLICY_MAX, "bigint"),
			func(c *Call) {
				c.DoAddFloat64(0, "key", 1.0)
			},
			false,
		},
		{
			"set_min_int64 golden path",
			newTestCall(pbsubstreams.Module_KindStore_UPDATE_POLICY_MIN, "int64"),
			func(c *Call) {
				c.DoSetMinInt64(0, "key", 1)
			},
			true,
		},
		{
			"set_min_int64 wrong type",
			newTestCall(pbsubstreams.Module_KindStore_UPDATE_POLICY_MIN, "float64"),
			func(c *Call) {
				c.DoSetMinInt64(0, "key", 1)
			},
			false,
		},
		{
			"set_min_int64 wrong policy",
			newTestCall(pbsubstreams.Module_KindStore_UPDATE_POLICY_MAX, "int64"),
			func(c *Call) {
				c.DoSetMinInt64(0, "key", 1)
			},
			false,
		},
		{
			"set_min_int64 wrong policy, type",
			newTestCall(pbsubstreams.Module_KindStore_UPDATE_POLICY_MAX, "float64"),
			func(c *Call) {
				c.DoSetMinInt64(0, "key", 1)
			},
			false,
		},
		{
			"set_min_bigint golden path",
			newTestCall(pbsubstreams.Module_KindStore_UPDATE_POLICY_MIN, "bigint"),
			func(c *Call) {
				c.DoSetMinBigInt(0, "key", "1")
			},
			true,
		},
		{
			"set_min_bigint wrong type",
			newTestCall(pbsubstreams.Module_KindStore_UPDATE_POLICY_MIN, "int64"),
			func(c *Call) {
				c.DoSetMinBigInt(0, "key", "1")
			},
			false,
		},
		{
			"set_min_bigint wrong policy",
			newTestCall(pbsubstreams.Module_KindStore_UPDATE_POLICY_MAX, "bigint"),
			func(c *Call) {
				c.DoSetMinBigInt(0, "key", "1")
			},
			false,
		},
		{
			"set_min_bigint wrong policy, type",
			newTestCall(pbsubstreams.Module_KindStore_UPDATE_POLICY_MAX, "bigdecimal"),
			func(c *Call) {
				c.DoSetMinBigInt(0, "key", "1")
			},
			false,
		},
		{
			"set_min_float64 golden path",
			newTestCall(pbsubstreams.Module_KindStore_UPDATE_POLICY_MIN, "float64"),
			func(c *Call) {
				c.DoSetMinFloat64(0, "key", 1.0)
			},
			true,
		},
		{
			"set_min_float64 wrong type",
			newTestCall(pbsubstreams.Module_KindStore_UPDATE_POLICY_MIN, "bigdecimal"),
			func(c *Call) {
				c.DoSetMinFloat64(0, "key", 1.0)
			},
			false,
		},
		{
			"set_min_float64 wrong policy",
			newTestCall(pbsubstreams.Module_KindStore_UPDATE_POLICY_MAX, "float64"),
			func(c *Call) {
				c.DoSetMinFloat64(0, "key", 1.0)
			},
			false,
		},
		{
			"set_min_float64 wrong policy, type",
			newTestCall(pbsubstreams.Module_KindStore_UPDATE_POLICY_MAX, "bigint"),
			func(c *Call) {
				c.DoSetMinFloat64(0, "key", 1.0)
			},
			false,
		},
		{
			"set_min_bigdecimal golden path",
			newTestCall(pbsubstreams.Module_KindStore_UPDATE_POLICY_MIN, "bigdecimal"),
			func(c *Call) {
				c.DoSetMinBigDecimal(0, "key", "1.0")
			},
			true,
		},
		{
			"set_min_bigdecimal wrong type",
			newTestCall(pbsubstreams.Module_KindStore_UPDATE_POLICY_MIN, "bigint"),
			func(c *Call) {
				c.DoSetMinBigDecimal(0, "key", "1.0")
			},
			false,
		},
		{
			"set_min_bigdecimal wrong policy",
			newTestCall(pbsubstreams.Module_KindStore_UPDATE_POLICY_MAX, "bigdecimal"),
			func(c *Call) {
				c.DoSetMinBigDecimal(0, "key", "1.0")
			},
			false,
		},
		{
			"set_min_bigdecimal wrong policy, type",
			newTestCall(pbsubstreams.Module_KindStore_UPDATE_POLICY_MAX, "float64"),
			func(c *Call) {
				c.DoSetMinBigDecimal(0, "key", "1.0")
			},
			false,
		},
		{
			"set_max_int64 golden path",
			newTestCall(pbsubstreams.Module_KindStore_UPDATE_POLICY_MAX, "int64"),
			func(c *Call) {
				c.DoSetMaxInt64(0, "key", 1)
			},
			true,
		},
		{
			"set_max_int64 wrong type",
			newTestCall(pbsubstreams.Module_KindStore_UPDATE_POLICY_MAX, "bigint"),
			func(c *Call) {
				c.DoSetMaxInt64(0, "key", 1)
			},
			false,
		},
		{
			"set_max_int64 wrong policy",
			newTestCall(pbsubstreams.Module_KindStore_UPDATE_POLICY_MIN, "int64"),
			func(c *Call) {
				c.DoSetMaxInt64(0, "key", 1)
			},
			false,
		},
		{
			"set_max_int64 wrong policy, type",
			newTestCall(pbsubstreams.Module_KindStore_UPDATE_POLICY_ADD, "float64"),
			func(c *Call) {
				c.DoSetMaxInt64(0, "key", 1)
			},
			false,
		},
		{
			"set_max_bigint golden path",
			newTestCall(pbsubstreams.Module_KindStore_UPDATE_POLICY_MAX, "bigint"),
			func(c *Call) {
				c.DoSetMaxBigInt(0, "key", "1")
			},
			true,
		},
		{
			"set_max_bigint wrong type",
			newTestCall(pbsubstreams.Module_KindStore_UPDATE_POLICY_MAX, "int64"),
			func(c *Call) {
				c.DoSetMaxBigInt(0, "key", "1")
			},
			false,
		},
		{
			"set_max_bigint wrong policy",
			newTestCall(pbsubstreams.Module_KindStore_UPDATE_POLICY_MIN, "bigint"),
			func(c *Call) {
				c.DoSetMaxBigInt(0, "key", "1")
			},
			false,
		},
		{
			"set_max_bigint wrong policy, type",
			newTestCall(pbsubstreams.Module_KindStore_UPDATE_POLICY_ADD, "float64"),
			func(c *Call) {
				c.DoSetMaxBigInt(0, "key", "1")
			},
			false,
		},
		{
			"set_max_float64 golden path",
			newTestCall(pbsubstreams.Module_KindStore_UPDATE_POLICY_MAX, "float64"),
			func(c *Call) {
				c.DoSetMaxFloat64(0, "key", 1.0)
			},
			true,
		},
		{
			"set_max_float64 wrong type",
			newTestCall(pbsubstreams.Module_KindStore_UPDATE_POLICY_MAX, "bigdecimal"),
			func(c *Call) {
				c.DoSetMaxFloat64(0, "key", 1.0)
			},
			false,
		},
		{
			"set_max_float64 wrong policy",
			newTestCall(pbsubstreams.Module_KindStore_UPDATE_POLICY_MIN, "int64"),
			func(c *Call) {
				c.DoSetMaxFloat64(0, "key", 1.0)
			},
			false,
		},
		{
			"set_max_float64 wrong policy, type",
			newTestCall(pbsubstreams.Module_KindStore_UPDATE_POLICY_ADD, "bigdecimal"),
			func(c *Call) {
				c.DoSetMaxFloat64(0, "key", 1.0)
			},
			false,
		},
		{
			"set_max_bigdecimal golden path",
			newTestCall(pbsubstreams.Module_KindStore_UPDATE_POLICY_MAX, "bigdecimal"),
			func(c *Call) {
				c.DoSetMaxBigDecimal(0, "key", "1.0")
			},
			true,
		},
		{
			"set_max_bigdecimal wrong type",
			newTestCall(pbsubstreams.Module_KindStore_UPDATE_POLICY_MAX, "float64"),
			func(c *Call) {
				c.DoSetMaxBigDecimal(0, "key", "1.0")
			},
			false,
		},
		{
			"set_max_bigdecimal wrong policy",
			newTestCall(pbsubstreams.Module_KindStore_UPDATE_POLICY_MIN, "int64"),
			func(c *Call) {
				c.DoSetMaxBigDecimal(0, "key", "1.0")
			},
			false,
		},
		{
			"set_max_bigdecimal wrong policy, type",
			newTestCall(pbsubstreams.Module_KindStore_UPDATE_POLICY_ADD, "float64"),
			func(c *Call) {
				c.DoSetMaxBigDecimal(0, "key", "1.0")
			},
			false,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			expectPanic(t, test.expectPanic, test.instance, test.testFunc)
		})
	}
}

func newTestCall(updatePolicy pbsubstreams.Module_KindStore_UpdatePolicy, valueType string) *Call {
	myStore := dstore.NewMockStore(nil)
	storeConf, err := store.NewConfig("test", 0, "", updatePolicy, valueType, myStore)
	if err != nil {
		panic("failed")
	}
	outStore := storeConf.NewFullKV(zap.NewNop())
	return &Call{updatePolicy: updatePolicy, valueType: valueType, outputStore: outStore, stats: metrics.NewReqStats(&metrics.Config{}, zap.NewNop())}
}

func expectPanic(t *testing.T, shouldPanic bool, c *Call, f func(c *Call)) {
	if shouldPanic {
		assert.NotPanics(t, func() { f(c) })
	} else {
		assert.Panics(t, func() { f(c) })
	}
}
