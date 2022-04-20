package state

import (
	"testing"

	pbtransform "github.com/streamingfast/substreams/pb/sf/substreams/transform/v1"

	"github.com/stretchr/testify/assert"
)

func TestMergeValues(t *testing.T) {
	b := &Builder{
		KV: map[string][]byte{},
		Store: &Store{
			ModuleHash:       "abc123",
			ModuleStartBlock: 1023,
			Name:             "testStore",
		},
		valueType:    OutputValueTypeInt64,
		updatePolicy: pbtransform.KindStore_UPDATE_POLICY_SUM,
	}

	b.writeMergeValues()
	_, ok := b.KV[valueTypeKey]
	assert.True(t, ok)
	_, ok = b.KV[updatePolicyKey]
	assert.True(t, ok)
	_, ok = b.KV[moduleStartBlockKey]
	assert.True(t, ok)
	_, ok = b.KV[moduleHashKey]
	assert.True(t, ok)
	_, ok = b.KV[storeNameKey]
	assert.True(t, ok)
}

func TestBuilder_Merge(t *testing.T) {
	tests := []struct {
		name            string
		latest          *Builder
		latestKV        map[string][]byte
		prev            *Builder
		prevKV          map[string][]byte
		expectedError   bool
		expectedKV      map[string][]byte
		deletedPrefixes []string
	}{
		{
			name:          "incompatible merge strategies",
			latest:        NewBuilder("b1", 0, pbtransform.KindStore_UPDATE_POLICY_IGNORE, OutputValueTypeString, nil),
			prev:          NewBuilder("b2", 0, pbtransform.KindStore_UPDATE_POLICY_REPLACE, OutputValueTypeString, nil),
			expectedError: true,
		},
		{
			name:          "incompatible value types",
			latest:        NewBuilder("b1", 0, pbtransform.KindStore_UPDATE_POLICY_IGNORE, OutputValueTypeString, nil),
			prev:          NewBuilder("b2", 0, pbtransform.KindStore_UPDATE_POLICY_IGNORE, OutputValueTypeBigFloat, nil),
			expectedError: true,
		},
		{
			name:   "replace (latest wins)",
			latest: NewBuilder("b1", 0, pbtransform.KindStore_UPDATE_POLICY_REPLACE, OutputValueTypeString, nil),
			latestKV: map[string][]byte{
				"one": []byte("foo"),
				"two": []byte("bar"),
			},
			prev: NewBuilder("b2", 0, pbtransform.KindStore_UPDATE_POLICY_REPLACE, OutputValueTypeString, nil),
			prevKV: map[string][]byte{
				"one":   []byte("baz"),
				"three": []byte("lol"),
			},
			expectedError: false,
			expectedKV: map[string][]byte{
				"one":   []byte("foo"),
				"two":   []byte("bar"),
				"three": []byte("lol"),
			},
		},
		{
			name:   "ignore (previous wins)",
			latest: NewBuilder("b1", 0, pbtransform.KindStore_UPDATE_POLICY_IGNORE, OutputValueTypeString, nil),
			latestKV: map[string][]byte{
				"one": []byte("foo"),
				"two": []byte("bar"),
			},
			prev: NewBuilder("b2", 0, pbtransform.KindStore_UPDATE_POLICY_IGNORE, OutputValueTypeString, nil),
			prevKV: map[string][]byte{
				"one":   []byte("baz"),
				"three": []byte("lol"),
			},
			expectedError: false,
			expectedKV: map[string][]byte{
				"one":   []byte("baz"),
				"two":   []byte("bar"),
				"three": []byte("lol"),
			},
		},
		{
			name:   "sum_int",
			latest: NewBuilder("b1", 0, pbtransform.KindStore_UPDATE_POLICY_SUM, OutputValueTypeInt64, nil),
			latestKV: map[string][]byte{
				"one": []byte("1"),
				"two": []byte("2"),
			},
			prev: NewBuilder("b2", 0, pbtransform.KindStore_UPDATE_POLICY_SUM, OutputValueTypeInt64, nil),
			prevKV: map[string][]byte{
				"one":   []byte("1"),
				"three": []byte("3"),
			},
			expectedError: false,
			expectedKV: map[string][]byte{
				"one":   []byte("2"),
				"two":   []byte("2"),
				"three": []byte("3"),
			},
		},
		{
			name:   "sum_big_int",
			latest: NewBuilder("b1", 0, pbtransform.KindStore_UPDATE_POLICY_SUM, OutputValueTypeBigInt, nil),
			latestKV: map[string][]byte{
				"one": []byte("1"),
				"two": []byte("2"),
			},
			prev: NewBuilder("b2", 0, pbtransform.KindStore_UPDATE_POLICY_SUM, OutputValueTypeBigInt, nil),
			prevKV: map[string][]byte{
				"one":   []byte("1"),
				"three": []byte("3"),
			},
			expectedError: false,
			expectedKV: map[string][]byte{
				"one":   []byte("2"),
				"two":   []byte("2"),
				"three": []byte("3"),
			},
		},
		{
			name:   "min_int",
			latest: NewBuilder("b1", 0, pbtransform.KindStore_UPDATE_POLICY_MIN, OutputValueTypeInt64, nil),
			latestKV: map[string][]byte{
				"one": []byte("1"),
				"two": []byte("2"),
			},
			prev: NewBuilder("b2", 0, pbtransform.KindStore_UPDATE_POLICY_MIN, OutputValueTypeInt64, nil),
			prevKV: map[string][]byte{
				"one":   []byte("2"),
				"three": []byte("3"),
			},
			expectedError: false,
			expectedKV: map[string][]byte{
				"one":   []byte("1"),
				"two":   []byte("2"),
				"three": []byte("3"),
			},
		},
		{
			name:   "min_big_int",
			latest: NewBuilder("b1", 0, pbtransform.KindStore_UPDATE_POLICY_MIN, OutputValueTypeBigInt, nil),
			latestKV: map[string][]byte{
				"one": []byte("1"),
				"two": []byte("2"),
			},
			prev: NewBuilder("b2", 0, pbtransform.KindStore_UPDATE_POLICY_MIN, OutputValueTypeBigInt, nil),
			prevKV: map[string][]byte{
				"one":   []byte("2"),
				"three": []byte("3"),
			},
			expectedError: false,
			expectedKV: map[string][]byte{
				"one":   []byte("1"),
				"two":   []byte("2"),
				"three": []byte("3"),
			},
		},
		{
			name:   "max_int",
			latest: NewBuilder("b1", 0, pbtransform.KindStore_UPDATE_POLICY_MAX, OutputValueTypeInt64, nil),
			latestKV: map[string][]byte{
				"one": []byte("1"),
				"two": []byte("2"),
			},
			prev: NewBuilder("b2", 0, pbtransform.KindStore_UPDATE_POLICY_MAX, OutputValueTypeInt64, nil),
			prevKV: map[string][]byte{
				"one":   []byte("2"),
				"three": []byte("3"),
			},
			expectedError: false,
			expectedKV: map[string][]byte{
				"one":   []byte("2"),
				"two":   []byte("2"),
				"three": []byte("3"),
			},
		},
		{
			name:   "max_big_int",
			latest: NewBuilder("b1", 0, pbtransform.KindStore_UPDATE_POLICY_MAX, OutputValueTypeBigInt, nil),
			latestKV: map[string][]byte{
				"one": []byte("1"),
				"two": []byte("2"),
			},
			prev: NewBuilder("b2", 0, pbtransform.KindStore_UPDATE_POLICY_MAX, OutputValueTypeBigInt, nil),
			prevKV: map[string][]byte{
				"one":   []byte("2"),
				"three": []byte("3"),
			},
			expectedError: false,
			expectedKV: map[string][]byte{
				"one":   []byte("2"),
				"two":   []byte("2"),
				"three": []byte("3"),
			},
		},
		{
			name:   "sum_float",
			latest: NewBuilder("b1", 0, pbtransform.KindStore_UPDATE_POLICY_SUM, OutputValueTypeFloat64, nil),
			latestKV: map[string][]byte{
				"one": []byte("10.1"),
				"two": []byte("20.1"),
			},
			prev: NewBuilder("b2", 0, pbtransform.KindStore_UPDATE_POLICY_SUM, OutputValueTypeFloat64, nil),
			prevKV: map[string][]byte{
				"one":   []byte("10.1"),
				"three": []byte("30.1"),
			},
			expectedError: false,
			expectedKV: map[string][]byte{
				"one":   []byte("20.2"),
				"two":   []byte("20.1"),
				"three": []byte("30.1"),
			},
		},
		{
			name:   "sum_big_float",
			latest: NewBuilder("b1", 0, pbtransform.KindStore_UPDATE_POLICY_SUM, OutputValueTypeBigFloat, nil),
			latestKV: map[string][]byte{
				"one": []byte("10.1"),
				"two": []byte("20.1"),
			},
			prev: NewBuilder("b2", 0, pbtransform.KindStore_UPDATE_POLICY_SUM, OutputValueTypeBigFloat, nil),
			prevKV: map[string][]byte{
				"one":   []byte("10.1"),
				"three": []byte("30.1"),
			},
			expectedError: false,
			expectedKV: map[string][]byte{
				"one":   []byte("20.2"),
				"two":   []byte("20.1"),
				"three": []byte("30.1"),
			},
		},
		{
			name:   "min_float",
			latest: NewBuilder("b1", 0, pbtransform.KindStore_UPDATE_POLICY_MIN, OutputValueTypeFloat64, nil),
			latestKV: map[string][]byte{
				"one": []byte("10.1"),
				"two": []byte("20.1"),
			},
			prev: NewBuilder("b2", 0, pbtransform.KindStore_UPDATE_POLICY_MIN, OutputValueTypeFloat64, nil),
			prevKV: map[string][]byte{
				"one":   []byte("20.1"),
				"three": []byte("30.1"),
			},
			expectedError: false,
			expectedKV: map[string][]byte{
				"one":   []byte("10.1"),
				"two":   []byte("20.1"),
				"three": []byte("30.1"),
			},
		},
		{
			name:   "min_big_float",
			latest: NewBuilder("b1", 0, pbtransform.KindStore_UPDATE_POLICY_MIN, OutputValueTypeBigFloat, nil),
			latestKV: map[string][]byte{
				"one": []byte("10.1"),
				"two": []byte("20.1"),
			},
			prev: NewBuilder("b2", 0, pbtransform.KindStore_UPDATE_POLICY_MIN, OutputValueTypeBigFloat, nil),
			prevKV: map[string][]byte{
				"one":   []byte("20.1"),
				"three": []byte("30.1"),
			},
			expectedError: false,
			expectedKV: map[string][]byte{
				"one":   []byte("10.1"),
				"two":   []byte("20.1"),
				"three": []byte("30.1"),
			},
		},
		{
			name:   "max_float",
			latest: NewBuilder("b1", 0, pbtransform.KindStore_UPDATE_POLICY_MAX, OutputValueTypeFloat64, nil),
			latestKV: map[string][]byte{
				"one": []byte("10.1"),
				"two": []byte("20.1"),
			},
			prev: NewBuilder("b2", 0, pbtransform.KindStore_UPDATE_POLICY_MAX, OutputValueTypeFloat64, nil),
			prevKV: map[string][]byte{
				"one":   []byte("20.1"),
				"three": []byte("30.1"),
			},
			expectedError: false,
			expectedKV: map[string][]byte{
				"one":   []byte("20.1"),
				"two":   []byte("20.1"),
				"three": []byte("30.1"),
			},
		},
		{
			name:   "max_big_float",
			latest: NewBuilder("b1", 0, pbtransform.KindStore_UPDATE_POLICY_MAX, OutputValueTypeBigFloat, nil),
			latestKV: map[string][]byte{
				"one": []byte("10.1"),
				"two": []byte("20.1"),
			},
			prev: NewBuilder("b2", 0, pbtransform.KindStore_UPDATE_POLICY_MAX, OutputValueTypeBigFloat, nil),
			prevKV: map[string][]byte{
				"one":   []byte("20.1"),
				"three": []byte("30.1"),
			},
			expectedError: false,
			expectedKV: map[string][]byte{
				"one":   []byte("20.1"),
				"two":   []byte("20.1"),
				"three": []byte("30.1"),
			},
		},
		{
			name:   "delete key prefixes",
			latest: NewBuilder("b1", 0, pbtransform.KindStore_UPDATE_POLICY_REPLACE, OutputValueTypeString, nil),
			latestKV: map[string][]byte{
				"t:1": []byte("bar"),
			},
			deletedPrefixes: []string{"p:"},
			prev:            NewBuilder("b2", 0, pbtransform.KindStore_UPDATE_POLICY_REPLACE, OutputValueTypeString, nil),
			prevKV: map[string][]byte{
				"t:1": []byte("baz"),
				"p:3": []byte("lol"),
			},
			expectedError: false,
			expectedKV: map[string][]byte{
				"t:1": []byte("bar"),
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			test.prev.partialMode = true
			test.latest.KV = test.latestKV
			test.prev.KV = test.prevKV
			test.latest.DeletedPrefixes = test.deletedPrefixes

			err := test.latest.Merge(test.prev)
			if err != nil && !test.expectedError {
				if !test.expectedError {
					t.Errorf("got unexpected error in test %s: %s", test.name, err.Error())
				}
				return
			}

			// check result both ways

			for k, v := range test.latest.KV {
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
					expected, _ := foundOrZeroBigFloat(test.latest.KV[k], true).Float64()
					assert.InDelta(t, actual, expected, 0.01)
				} else {
					expected := string(test.latest.KV[k])
					actual := string(v)
					assert.Equal(t, expected, actual)
				}
			}
		})
	}
}
