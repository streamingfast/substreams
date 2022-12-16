package store

import (
	"math/big"
	"strconv"
	"testing"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/stretchr/testify/assert"
)

func TestStoreSumBigInt(t *testing.T) {
	tests := []struct {
		name          string
		key           string
		existingValue []byte
		value         *big.Int
		expectedValue *big.Int
	}{
		{
			name:          "found",
			key:           "key",
			existingValue: []byte("3"),
			value:         big.NewInt(4),
			expectedValue: big.NewInt(7),
		},
		{
			name:          "not found",
			key:           "key",
			existingValue: nil,
			value:         big.NewInt(4),
			expectedValue: big.NewInt(4),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			b := newTestBaseStore(t, pbsubstreams.Module_KindStore_UPDATE_POLICY_UNSET, "", nil)
			if test.existingValue != nil {
				b.kv[test.key] = test.existingValue
				b.totalSizeBytes += uint64(len(test.key) + len(test.existingValue))
			}

			b.SumBigInt(0, test.key, test.value)
			actual, found := b.GetAt(0, test.key)
			if !found {
				t.Errorf("value not found")
			}

			actualInt, ok := new(big.Int).SetString(string(actual), 10)
			assert.True(t, ok)
			assert.Equal(t, 0, actualInt.Cmp(test.expectedValue))
		})
	}
}

func TestStoreSumInt64(t *testing.T) {
	tests := []struct {
		name          string
		key           string
		existingValue []byte
		value         int64
		expectedValue int64
	}{
		{
			name:          "found",
			key:           "key",
			existingValue: []byte("3"),
			value:         4,
			expectedValue: 7,
		},
		{
			name:          "not found",
			key:           "key",
			existingValue: nil,
			value:         4,
			expectedValue: 4,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			b := newTestBaseStore(t, pbsubstreams.Module_KindStore_UPDATE_POLICY_UNSET, "", nil)
			if test.existingValue != nil {
				b.kv[test.key] = test.existingValue
				b.totalSizeBytes += uint64(len(test.key) + len(test.existingValue))
			}

			b.SumInt64(0, test.key, test.value)
			actual, found := b.GetAt(0, test.key)
			if !found {
				t.Errorf("value not found")
			}

			actualInt, err := strconv.ParseInt(string(actual), 10, 64)
			assert.NoError(t, err)

			assert.Equal(t, test.expectedValue, actualInt)
		})
	}
}

func TestStoreSumFloat64(t *testing.T) {
	tests := []struct {
		name          string
		key           string
		existingValue []byte
		value         float64
		expectedValue float64
	}{
		{
			name:          "found",
			key:           "key",
			existingValue: []byte("3.0"),
			value:         4.0,
			expectedValue: 7.0,
		},
		{
			name:          "not found",
			key:           "key",
			existingValue: nil,
			value:         4.0,
			expectedValue: 4.0,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			b := newTestBaseStore(t, pbsubstreams.Module_KindStore_UPDATE_POLICY_UNSET, "", nil)
			if test.existingValue != nil {
				b.kv[test.key] = test.existingValue
				b.totalSizeBytes += uint64(len(test.key) + len(test.existingValue))
			}

			b.SumFloat64(0, test.key, test.value)
			actual, found := b.GetAt(0, test.key)
			if !found {
				t.Errorf("value not found")
			}

			actualInt, err := strconv.ParseFloat(string(actual), 64)
			assert.NoError(t, err)

			assert.Equal(t, test.expectedValue, actualInt)
		})
	}
}

func TestStoreSumBigFloat(t *testing.T) {
	tests := []struct {
		name          string
		key           string
		existingValue []byte
		value         *big.Float
		expectedValue *big.Float
	}{
		{
			name:          "found",
			key:           "key",
			existingValue: []byte("3.0"),
			value:         big.NewFloat(4),
			expectedValue: big.NewFloat(7),
		},
		{
			name:          "not found",
			key:           "key",
			existingValue: nil,
			value:         big.NewFloat(4),
			expectedValue: big.NewFloat(4),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			b := newTestBaseStore(t, pbsubstreams.Module_KindStore_UPDATE_POLICY_UNSET, "", nil)
			if test.existingValue != nil {
				b.setKV(test.key, test.existingValue)
			}

			b.SumBigDecimal(0, test.key, test.value)
			actual, found := b.GetAt(0, test.key)
			if !found {
				t.Errorf("value not found")
			}

			actualInt, _, err := big.ParseFloat(string(actual), 10, 100, big.ToNearestEven)
			assert.NoError(t, err)

			assert.Equal(t, 0, actualInt.Cmp(test.expectedValue))
		})
	}
}
