package store

import (
	"github.com/stretchr/testify/require"
	"testing"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/stretchr/testify/assert"
)

func TestStore_Merge(t *testing.T) {
	tests := []struct {
		name          string
		latest        *PartialKV
		prev          *FullKV
		expectedError bool
		expectedKV    map[string][]byte
	}{
		{
			name:          "incompatible merge strategies",
			latest:        newPartialStore(map[string][]byte{}, pbsubstreams.Module_KindStore_UPDATE_POLICY_SET_IF_NOT_EXISTS, OutputValueTypeString, nil),
			prev:          newStore(map[string][]byte{}, pbsubstreams.Module_KindStore_UPDATE_POLICY_SET, OutputValueTypeString),
			expectedError: true,
		},
		{
			name:          "incompatible value types",
			latest:        newPartialStore(map[string][]byte{}, pbsubstreams.Module_KindStore_UPDATE_POLICY_SET_IF_NOT_EXISTS, OutputValueTypeString, nil),
			prev:          newStore(map[string][]byte{}, pbsubstreams.Module_KindStore_UPDATE_POLICY_SET_IF_NOT_EXISTS, OutputValueTypeBigFloat),
			expectedError: true,
		},
		{
			name: "replace (latest wins)",
			latest: newPartialStore(
				map[string][]byte{
					"one": []byte("foo"),
					"two": []byte("bar"),
				},
				pbsubstreams.Module_KindStore_UPDATE_POLICY_SET,
				OutputValueTypeString,
				nil,
			),
			prev: newStore(
				map[string][]byte{
					"one":   []byte("baz"),
					"three": []byte("lol"),
				},
				pbsubstreams.Module_KindStore_UPDATE_POLICY_SET,
				OutputValueTypeString,
			),
			expectedError: false,
			expectedKV: map[string][]byte{
				"one":   []byte("foo"),
				"two":   []byte("bar"),
				"three": []byte("lol"),
			},
		},
		{
			name: "ignore (previous wins)",
			latest: newPartialStore(map[string][]byte{
				"one": []byte("foo"),
				"two": []byte("bar"),
			}, pbsubstreams.Module_KindStore_UPDATE_POLICY_SET_IF_NOT_EXISTS, OutputValueTypeString, nil),
			prev: newStore(map[string][]byte{
				"one":   []byte("baz"),
				"three": []byte("lol"),
			}, pbsubstreams.Module_KindStore_UPDATE_POLICY_SET_IF_NOT_EXISTS, OutputValueTypeString),
			expectedError: false,
			expectedKV: map[string][]byte{
				"one":   []byte("baz"),
				"two":   []byte("bar"),
				"three": []byte("lol"),
			},
		},
		{
			name: "append",
			latest: newPartialStore(map[string][]byte{
				"one": []byte("foo;"),
				"two": []byte("bar;"),
			}, pbsubstreams.Module_KindStore_UPDATE_POLICY_APPEND, OutputValueTypeString, nil),
			prev: newStore(map[string][]byte{
				"one":   []byte("baz;"),
				"three": []byte("lol;"),
			}, pbsubstreams.Module_KindStore_UPDATE_POLICY_APPEND, OutputValueTypeString),
			expectedError: false,
			expectedKV: map[string][]byte{
				"one":   []byte("baz;foo;"),
				"two":   []byte("bar;"),
				"three": []byte("lol;"),
			},
		},
		{
			name: "sum_int",
			latest: newPartialStore(map[string][]byte{
				"one": []byte("1"),
				"two": []byte("2"),
			}, pbsubstreams.Module_KindStore_UPDATE_POLICY_ADD, OutputValueTypeInt64, nil),
			prev: newStore(map[string][]byte{
				"one":   []byte("1"),
				"three": []byte("3"),
			}, pbsubstreams.Module_KindStore_UPDATE_POLICY_ADD, OutputValueTypeInt64),
			expectedError: false,
			expectedKV: map[string][]byte{
				"one":   []byte("2"),
				"two":   []byte("2"),
				"three": []byte("3"),
			},
		},
		{
			name: "sum_big_int",
			latest: newPartialStore(map[string][]byte{
				"one": []byte("1"),
				"two": []byte("2"),
			}, pbsubstreams.Module_KindStore_UPDATE_POLICY_ADD, OutputValueTypeBigInt, nil),
			prev: newStore(map[string][]byte{
				"one":   []byte("1"),
				"three": []byte("3"),
			}, pbsubstreams.Module_KindStore_UPDATE_POLICY_ADD, OutputValueTypeBigInt),
			expectedError: false,
			expectedKV: map[string][]byte{
				"one":   []byte("2"),
				"two":   []byte("2"),
				"three": []byte("3"),
			},
		},
		{
			name: "min_int",
			latest: newPartialStore(
				map[string][]byte{
					"one": []byte("1"),
					"two": []byte("2"),
				}, pbsubstreams.Module_KindStore_UPDATE_POLICY_MIN, OutputValueTypeInt64, nil),
			prev: newStore(map[string][]byte{
				"one":   []byte("2"),
				"three": []byte("3"),
			}, pbsubstreams.Module_KindStore_UPDATE_POLICY_MIN, OutputValueTypeInt64),
			expectedError: false,
			expectedKV: map[string][]byte{
				"one":   []byte("1"),
				"two":   []byte("2"),
				"three": []byte("3"),
			},
		},
		{
			name: "min_big_int",
			latest: newPartialStore(
				map[string][]byte{
					"one": []byte("1"),
					"two": []byte("2"),
				}, pbsubstreams.Module_KindStore_UPDATE_POLICY_MIN, OutputValueTypeBigInt, nil),
			prev: newStore(
				map[string][]byte{
					"one":   []byte("2"),
					"three": []byte("3"),
				}, pbsubstreams.Module_KindStore_UPDATE_POLICY_MIN, OutputValueTypeBigInt),
			expectedError: false,
			expectedKV: map[string][]byte{
				"one":   []byte("1"),
				"two":   []byte("2"),
				"three": []byte("3"),
			},
		},
		{
			name: "max_int",
			latest: newPartialStore(
				map[string][]byte{
					"one": []byte("1"),
					"two": []byte("2"),
				}, pbsubstreams.Module_KindStore_UPDATE_POLICY_MAX, OutputValueTypeInt64, nil),
			prev: newStore(map[string][]byte{
				"one":   []byte("2"),
				"three": []byte("3"),
			}, pbsubstreams.Module_KindStore_UPDATE_POLICY_MAX, OutputValueTypeInt64),
			expectedError: false,
			expectedKV: map[string][]byte{
				"one":   []byte("2"),
				"two":   []byte("2"),
				"three": []byte("3"),
			},
		},
		{
			name: "max_big_int",
			latest: newPartialStore(map[string][]byte{
				"one": []byte("1"),
				"two": []byte("2"),
			}, pbsubstreams.Module_KindStore_UPDATE_POLICY_MAX, OutputValueTypeBigInt, nil),
			prev: newStore(map[string][]byte{
				"one":   []byte("2"),
				"three": []byte("3"),
			}, pbsubstreams.Module_KindStore_UPDATE_POLICY_MAX, OutputValueTypeBigInt),
			expectedError: false,
			expectedKV: map[string][]byte{
				"one":   []byte("2"),
				"two":   []byte("2"),
				"three": []byte("3"),
			},
		},
		{
			name: "sum_float",
			latest: newPartialStore(map[string][]byte{
				"one": []byte("10.1"),
				"two": []byte("20.1"),
			}, pbsubstreams.Module_KindStore_UPDATE_POLICY_ADD, OutputValueTypeFloat64, nil),
			prev: newStore(map[string][]byte{
				"one":   []byte("10.1"),
				"three": []byte("30.1"),
			}, pbsubstreams.Module_KindStore_UPDATE_POLICY_ADD, OutputValueTypeFloat64),
			expectedError: false,
			expectedKV: map[string][]byte{
				"one":   []byte("20.2"),
				"two":   []byte("20.1"),
				"three": []byte("30.1"),
			},
		},
		{
			name: "sum_big_float",
			latest: newPartialStore(map[string][]byte{
				"one": []byte("10.1"),
				"two": []byte("20.1"),
			}, pbsubstreams.Module_KindStore_UPDATE_POLICY_ADD, OutputValueTypeBigFloat, nil),
			prev: newStore(map[string][]byte{
				"one":   []byte("10.1"),
				"three": []byte("30.1"),
			}, pbsubstreams.Module_KindStore_UPDATE_POLICY_ADD, OutputValueTypeBigFloat),
			expectedError: false,
			expectedKV: map[string][]byte{
				"one":   []byte("20.2"),
				"two":   []byte("20.1"),
				"three": []byte("30.1"),
			},
		},
		{
			name: "min_float",
			latest: newPartialStore(map[string][]byte{
				"one": []byte("10.1"),
				"two": []byte("20.1"),
			}, pbsubstreams.Module_KindStore_UPDATE_POLICY_MIN, OutputValueTypeFloat64, nil),
			prev: newStore(map[string][]byte{
				"one":   []byte("20.1"),
				"three": []byte("30.1"),
			}, pbsubstreams.Module_KindStore_UPDATE_POLICY_MIN, OutputValueTypeFloat64),
			expectedError: false,
			expectedKV: map[string][]byte{
				"one":   []byte("10.1"),
				"two":   []byte("20.1"),
				"three": []byte("30.1"),
			},
		},
		{
			name: "min_big_float",
			latest: newPartialStore(map[string][]byte{
				"one": []byte("10.1"),
				"two": []byte("20.1"),
			}, pbsubstreams.Module_KindStore_UPDATE_POLICY_MIN, OutputValueTypeBigFloat, nil),
			prev: newStore(map[string][]byte{
				"one":   []byte("20.1"),
				"three": []byte("30.1"),
			}, pbsubstreams.Module_KindStore_UPDATE_POLICY_MIN, OutputValueTypeBigFloat),
			expectedError: false,
			expectedKV: map[string][]byte{
				"one":   []byte("10.1"),
				"two":   []byte("20.1"),
				"three": []byte("30.1"),
			},
		},
		{
			name: "max_float",
			latest: newPartialStore(map[string][]byte{
				"one": []byte("10.1"),
				"two": []byte("20.1"),
			}, pbsubstreams.Module_KindStore_UPDATE_POLICY_MAX, OutputValueTypeFloat64, nil),
			prev: newStore(map[string][]byte{
				"one":   []byte("20.1"),
				"three": []byte("30.1"),
			}, pbsubstreams.Module_KindStore_UPDATE_POLICY_MAX, OutputValueTypeFloat64),
			expectedError: false,
			expectedKV: map[string][]byte{
				"one":   []byte("20.1"),
				"two":   []byte("20.1"),
				"three": []byte("30.1"),
			},
		},
		{
			name: "max_big_float",
			latest: newPartialStore(map[string][]byte{
				"one": []byte("10.1"),
				"two": []byte("20.1"),
			}, pbsubstreams.Module_KindStore_UPDATE_POLICY_MAX, OutputValueTypeBigFloat, nil),
			prev: newStore(map[string][]byte{
				"one":   []byte("20.1"),
				"three": []byte("30.1"),
			}, pbsubstreams.Module_KindStore_UPDATE_POLICY_MAX, OutputValueTypeBigFloat),
			expectedError: false,
			expectedKV: map[string][]byte{
				"one":   []byte("20.1"),
				"two":   []byte("20.1"),
				"three": []byte("30.1"),
			},
		},
		{
			name: "delete key prefixes",
			latest: newPartialStore(
				map[string][]byte{
					"t:1": []byte("bar"),
				}, pbsubstreams.Module_KindStore_UPDATE_POLICY_SET, OutputValueTypeString, []string{"p:"}),
			prev: newStore(map[string][]byte{
				"t:1": []byte("baz"),
				"p:3": []byte("lol"),
			}, pbsubstreams.Module_KindStore_UPDATE_POLICY_SET, OutputValueTypeString),
			expectedError: false,
			expectedKV: map[string][]byte{
				"t:1": []byte("bar"),
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.prev.Merge(test.latest)

			if test.expectedError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			for k, v := range test.prev.kv {
				if test.latest.valueType == OutputValueTypeBigFloat {
					actual, _ := foundOrZeroBigFloat(v, true).Float64()
					expected, _ := foundOrZeroBigFloat(test.expectedKV[k], true).Float64()
					assert.InDelta(t, actual, expected, 0.01)
				} else {
					expected := string(test.expectedKV[k])
					actual := string(v)
					assert.Equal(t, expected, actual)
				}
			}

			for k, v := range test.expectedKV {
				if test.latest.valueType == OutputValueTypeBigFloat {
					actual, _ := foundOrZeroBigFloat(v, true).Float64()
					expected, _ := foundOrZeroBigFloat(test.prev.kv[k], true).Float64()
					assert.InDelta(t, actual, expected, 0.01)
				} else {
					expected := string(test.prev.kv[k])
					actual := string(v)
					assert.Equal(t, expected, actual)
				}
			}
		})
	}
}

func newPartialStore(kv map[string][]byte, updatePolicy pbsubstreams.Module_KindStore_UpdatePolicy, valueType string, deletedPrefixes []string) *PartialKV {
	b := &BaseStore{
		kv:           kv,
		updatePolicy: updatePolicy,
		valueType:    valueType,
	}

	return &PartialKV{BaseStore: b, DeletedPrefixes: deletedPrefixes}
}

func newStore(kv map[string][]byte, updatePolicy pbsubstreams.Module_KindStore_UpdatePolicy, valueType string) *FullKV {
	b := &BaseStore{
		kv:           kv,
		updatePolicy: updatePolicy,
		valueType:    valueType,
	}
	return &FullKV{BaseStore: b}
}
