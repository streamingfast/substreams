package store

import (
	"go.uber.org/zap"
	"testing"

	"github.com/streamingfast/substreams/manifest"
	"github.com/stretchr/testify/require"

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
			latest:        newPartialStore(map[string][]byte{}, pbsubstreams.Module_KindStore_UPDATE_POLICY_SET_IF_NOT_EXISTS, manifest.OutputValueTypeString, nil),
			prev:          newStore(map[string][]byte{}, pbsubstreams.Module_KindStore_UPDATE_POLICY_SET, manifest.OutputValueTypeString),
			expectedError: true,
		},
		{
			name:          "incompatible value types",
			latest:        newPartialStore(map[string][]byte{}, pbsubstreams.Module_KindStore_UPDATE_POLICY_SET_IF_NOT_EXISTS, manifest.OutputValueTypeString, nil),
			prev:          newStore(map[string][]byte{}, pbsubstreams.Module_KindStore_UPDATE_POLICY_SET_IF_NOT_EXISTS, manifest.OutputValueTypeBigDecimal),
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
				manifest.OutputValueTypeString,
				nil,
			),
			prev: newStore(
				map[string][]byte{
					"one":   []byte("baz"),
					"three": []byte("lol"),
				},
				pbsubstreams.Module_KindStore_UPDATE_POLICY_SET,
				manifest.OutputValueTypeString,
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
			}, pbsubstreams.Module_KindStore_UPDATE_POLICY_SET_IF_NOT_EXISTS, manifest.OutputValueTypeString, nil),
			prev: newStore(map[string][]byte{
				"one":   []byte("baz"),
				"three": []byte("lol"),
			}, pbsubstreams.Module_KindStore_UPDATE_POLICY_SET_IF_NOT_EXISTS, manifest.OutputValueTypeString),
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
			}, pbsubstreams.Module_KindStore_UPDATE_POLICY_APPEND, manifest.OutputValueTypeString, nil),
			prev: newStore(map[string][]byte{
				"one":   []byte("baz;"),
				"three": []byte("lol;"),
			}, pbsubstreams.Module_KindStore_UPDATE_POLICY_APPEND, manifest.OutputValueTypeString),
			expectedError: false,
			expectedKV: map[string][]byte{
				"one":   []byte("baz;foo;"),
				"two":   []byte("bar;"),
				"three": []byte("lol;"),
			},
		},
		{
			name: "append exceeds limit",
			latest: newPartialStore(map[string][]byte{
				"one": []byte("foo;"),
				"two": []byte("bar;"),
			}, pbsubstreams.Module_KindStore_UPDATE_POLICY_APPEND, manifest.OutputValueTypeString, nil),
			prev: newStore(map[string][]byte{
				"one":   []byte("baz;supercalifrastilisticexpialidocious;"),
				"three": []byte("lol;"),
			}, pbsubstreams.Module_KindStore_UPDATE_POLICY_APPEND, manifest.OutputValueTypeString),
			expectedError: true,
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
			}, pbsubstreams.Module_KindStore_UPDATE_POLICY_ADD, manifest.OutputValueTypeInt64, nil),
			prev: newStore(map[string][]byte{
				"one":   []byte("1"),
				"three": []byte("3"),
			}, pbsubstreams.Module_KindStore_UPDATE_POLICY_ADD, manifest.OutputValueTypeInt64),
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
			}, pbsubstreams.Module_KindStore_UPDATE_POLICY_ADD, manifest.OutputValueTypeBigInt, nil),
			prev: newStore(map[string][]byte{
				"one":   []byte("1"),
				"three": []byte("3"),
			}, pbsubstreams.Module_KindStore_UPDATE_POLICY_ADD, manifest.OutputValueTypeBigInt),
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
				}, pbsubstreams.Module_KindStore_UPDATE_POLICY_MIN, manifest.OutputValueTypeInt64, nil),
			prev: newStore(map[string][]byte{
				"one":   []byte("2"),
				"three": []byte("3"),
			}, pbsubstreams.Module_KindStore_UPDATE_POLICY_MIN, manifest.OutputValueTypeInt64),
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
				}, pbsubstreams.Module_KindStore_UPDATE_POLICY_MIN, manifest.OutputValueTypeBigInt, nil),
			prev: newStore(
				map[string][]byte{
					"one":   []byte("2"),
					"three": []byte("3"),
				}, pbsubstreams.Module_KindStore_UPDATE_POLICY_MIN, manifest.OutputValueTypeBigInt),
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
				}, pbsubstreams.Module_KindStore_UPDATE_POLICY_MAX, manifest.OutputValueTypeInt64, nil),
			prev: newStore(map[string][]byte{
				"one":   []byte("2"),
				"three": []byte("3"),
			}, pbsubstreams.Module_KindStore_UPDATE_POLICY_MAX, manifest.OutputValueTypeInt64),
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
			}, pbsubstreams.Module_KindStore_UPDATE_POLICY_MAX, manifest.OutputValueTypeBigInt, nil),
			prev: newStore(map[string][]byte{
				"one":   []byte("2"),
				"three": []byte("3"),
			}, pbsubstreams.Module_KindStore_UPDATE_POLICY_MAX, manifest.OutputValueTypeBigInt),
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
			}, pbsubstreams.Module_KindStore_UPDATE_POLICY_ADD, manifest.OutputValueTypeFloat64, nil),
			prev: newStore(map[string][]byte{
				"one":   []byte("10.1"),
				"three": []byte("30.1"),
			}, pbsubstreams.Module_KindStore_UPDATE_POLICY_ADD, manifest.OutputValueTypeFloat64),
			expectedError: false,
			expectedKV: map[string][]byte{
				"one":   []byte("20.2"),
				"two":   []byte("20.1"),
				"three": []byte("30.1"),
			},
		},
		{
			name: "sum_big_decimal",
			latest: newPartialStore(map[string][]byte{
				"one": []byte("10.1"),
				"two": []byte("20.1"),
			}, pbsubstreams.Module_KindStore_UPDATE_POLICY_ADD, manifest.OutputValueTypeBigDecimal, nil),
			prev: newStore(map[string][]byte{
				"one":   []byte("10.1"),
				"three": []byte("30.1"),
			}, pbsubstreams.Module_KindStore_UPDATE_POLICY_ADD, manifest.OutputValueTypeBigDecimal),
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
			}, pbsubstreams.Module_KindStore_UPDATE_POLICY_MIN, manifest.OutputValueTypeFloat64, nil),
			prev: newStore(map[string][]byte{
				"one":   []byte("20.1"),
				"three": []byte("30.1"),
			}, pbsubstreams.Module_KindStore_UPDATE_POLICY_MIN, manifest.OutputValueTypeFloat64),
			expectedError: false,
			expectedKV: map[string][]byte{
				"one":   []byte("10.1"),
				"two":   []byte("20.1"),
				"three": []byte("30.1"),
			},
		},
		{
			name: "min_big_decimal",
			latest: newPartialStore(map[string][]byte{
				"one": []byte("10.1"),
				"two": []byte("20.1"),
			}, pbsubstreams.Module_KindStore_UPDATE_POLICY_MIN, manifest.OutputValueTypeBigDecimal, nil),
			prev: newStore(map[string][]byte{
				"one":   []byte("20.1"),
				"three": []byte("30.1"),
			}, pbsubstreams.Module_KindStore_UPDATE_POLICY_MIN, manifest.OutputValueTypeBigDecimal),
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
			}, pbsubstreams.Module_KindStore_UPDATE_POLICY_MAX, manifest.OutputValueTypeFloat64, nil),
			prev: newStore(map[string][]byte{
				"one":   []byte("20.1"),
				"three": []byte("30.1"),
			}, pbsubstreams.Module_KindStore_UPDATE_POLICY_MAX, manifest.OutputValueTypeFloat64),
			expectedError: false,
			expectedKV: map[string][]byte{
				"one":   []byte("20.1"),
				"two":   []byte("20.1"),
				"three": []byte("30.1"),
			},
		},
		{
			name: "max_big_decimal",
			latest: newPartialStore(map[string][]byte{
				"one": []byte("10.1"),
				"two": []byte("20.1"),
			}, pbsubstreams.Module_KindStore_UPDATE_POLICY_MAX, manifest.OutputValueTypeBigDecimal, nil),
			prev: newStore(map[string][]byte{
				"one":   []byte("20.1"),
				"three": []byte("30.1"),
			}, pbsubstreams.Module_KindStore_UPDATE_POLICY_MAX, manifest.OutputValueTypeBigDecimal),
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
				}, pbsubstreams.Module_KindStore_UPDATE_POLICY_SET, manifest.OutputValueTypeString, []string{"p:"}),
			prev: newStore(map[string][]byte{
				"t:1": []byte("baz"),
				"p:3": []byte("lol"),
			}, pbsubstreams.Module_KindStore_UPDATE_POLICY_SET, manifest.OutputValueTypeString),
			expectedError: false,
			expectedKV: map[string][]byte{
				"t:1": []byte("bar"),
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.latest.updatePolicy == pbsubstreams.Module_KindStore_UPDATE_POLICY_APPEND {
				test.latest.appendLimit = 20
				test.prev.appendLimit = 20
			}

			err := test.prev.Merge(test.latest)
			if test.expectedError {
				require.Error(t, err)
				return
			} else {
				require.NoError(t, err)
			}

			for k, v := range test.prev.kv {
				if test.latest.valueType == manifest.OutputValueTypeBigDecimal {
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
				if test.latest.valueType == manifest.OutputValueTypeBigDecimal {
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
	b := &baseStore{
		kv: kv,
		Config: &Config{
			updatePolicy: updatePolicy,
			valueType:    valueType,
		},
	}

	return &PartialKV{baseStore: b, DeletedPrefixes: deletedPrefixes}
}

func newStore(kv map[string][]byte, updatePolicy pbsubstreams.Module_KindStore_UpdatePolicy, valueType string) *FullKV {
	b := &baseStore{
		kv: kv,
		Config: &Config{
			updatePolicy: updatePolicy,
			valueType:    valueType,
		},
		logger: zap.NewNop(),
	}
	return &FullKV{baseStore: b}
}
