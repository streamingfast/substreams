package state

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBuilder_Merge(t *testing.T) {
	tests := []struct {
		name          string
		latest        *Builder
		latestKV      map[string][]byte
		prev          *Builder
		prevKV        map[string][]byte
		expectedError bool
		expectedKV    map[string][]byte
	}{
		{
			name:          "incompatible merge strategies",
			latest:        NewBuilder("b1", UpdatePolicyIgnore, OutputValueTypeString, "", nil),
			prev:          NewBuilder("b2", UpdatePolicyReplace, OutputValueTypeString, "", nil),
			expectedError: true,
		},
		{
			name:          "incompatible value types",
			latest:        NewBuilder("b1", UpdatePolicyIgnore, OutputValueTypeString, "", nil),
			prev:          NewBuilder("b2", UpdatePolicyIgnore, OutputValueTypeBigFloat, "", nil),
			expectedError: true,
		},
		{
			name:   "replace (latest wins)",
			latest: NewBuilder("b1", UpdatePolicyReplace, OutputValueTypeString, "", nil),
			latestKV: map[string][]byte{
				"one": []byte("foo"),
				"two": []byte("bar"),
			},
			prev: NewBuilder("b2", UpdatePolicyReplace, OutputValueTypeString, "", nil),
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
			latest: NewBuilder("b1", UpdatePolicyIgnore, OutputValueTypeString, "", nil),
			latestKV: map[string][]byte{
				"one": []byte("foo"),
				"two": []byte("bar"),
			},
			prev: NewBuilder("b2", UpdatePolicyIgnore, OutputValueTypeString, "", nil),
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
			latest: NewBuilder("b1", UpdatePolicySum, OutputValueTypeInt64, "", nil),
			latestKV: map[string][]byte{
				"one": []byte("1"),
				"two": []byte("2"),
			},
			prev: NewBuilder("b2", UpdatePolicySum, OutputValueTypeInt64, "", nil),
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
			latest: NewBuilder("b1", UpdatePolicyMin, OutputValueTypeInt64, "", nil),
			latestKV: map[string][]byte{
				"one": []byte("1"),
				"two": []byte("2"),
			},
			prev: NewBuilder("b2", UpdatePolicyMin, OutputValueTypeInt64, "", nil),
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
			latest: NewBuilder("b1", UpdatePolicyMax, OutputValueTypeInt64, "", nil),
			latestKV: map[string][]byte{
				"one": []byte("1"),
				"two": []byte("2"),
			},
			prev: NewBuilder("b2", UpdatePolicyMax, OutputValueTypeInt64, "", nil),
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
			latest: NewBuilder("b1", UpdatePolicySum, OutputValueTypeBigFloat, "", nil),
			latestKV: map[string][]byte{
				"one": []byte("10.0"),
				"two": []byte("20.0"),
			},
			prev: NewBuilder("b2", UpdatePolicySum, OutputValueTypeBigFloat, "", nil),
			prevKV: map[string][]byte{
				"one":   []byte("10.0"),
				"three": []byte("30.0"),
			},
			expectedError: false,
			expectedKV: map[string][]byte{
				"one":   []byte("20.0"),
				"two":   []byte("20.0"),
				"three": []byte("30.0"),
			},
		},
		{
			name:   "min_float",
			latest: NewBuilder("b1", UpdatePolicyMin, OutputValueTypeBigFloat, "", nil),
			latestKV: map[string][]byte{
				"one": []byte("10.0"),
				"two": []byte("20.0"),
			},
			prev: NewBuilder("b2", UpdatePolicyMin, OutputValueTypeBigFloat, "", nil),
			prevKV: map[string][]byte{
				"one":   []byte("20.0"),
				"three": []byte("30.0"),
			},
			expectedError: false,
			expectedKV: map[string][]byte{
				"one":   []byte("10.0"),
				"two":   []byte("20.0"),
				"three": []byte("30.0"),
			},
		},
		{
			name:   "max_float",
			latest: NewBuilder("b1", UpdatePolicyMax, OutputValueTypeBigFloat, "", nil),
			latestKV: map[string][]byte{
				"one": []byte("10.0"),
				"two": []byte("20.0"),
			},
			prev: NewBuilder("b2", UpdatePolicyMax, OutputValueTypeBigFloat, "", nil),
			prevKV: map[string][]byte{
				"one":   []byte("20.0"),
				"three": []byte("30.0"),
			},
			expectedError: false,
			expectedKV: map[string][]byte{
				"one":   []byte("20.0"),
				"two":   []byte("20.0"),
				"three": []byte("30.0"),
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			test.latest.KV = test.latestKV
			test.prev.KV = test.prevKV

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
					actual, _ := foundOrZeroFloat(v, true).Float64()
					expected, _ := foundOrZeroFloat(test.expectedKV[k], true).Float64()
					assert.InDelta(t, actual, expected, 0.01)
				} else {
					expected := string(test.expectedKV[k])
					actual := string(v)
					assert.Equal(t, expected, actual)
				}
			}

			for k, v := range test.expectedKV {
				if test.latest.valueType == OutputValueTypeBigFloat {
					actual, _ := foundOrZeroFloat(v, true).Float64()
					expected, _ := foundOrZeroFloat(test.latest.KV[k], true).Float64()
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
