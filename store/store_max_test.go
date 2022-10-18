package store

import (
	"fmt"
	"math/big"
	"strconv"
	"testing"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"

	"github.com/stretchr/testify/assert"
)

func TestStoreSetMaxBigInt(t *testing.T) {
	tests := []struct {
		name          string
		store         *baseStore
		key           string
		existingValue *big.Int
		value         *big.Int
		expectedValue *big.Int
	}{
		{
			name:          "found less",
			store:         newTestBaseStore(t, pbsubstreams.Module_KindStore_UPDATE_POLICY_UNSET, "", nil),
			key:           "key",
			existingValue: big.NewInt(3),
			value:         big.NewInt(4),
			expectedValue: big.NewInt(4),
		},
		{
			name:          "found greater",
			store:         newTestBaseStore(t, pbsubstreams.Module_KindStore_UPDATE_POLICY_UNSET, "", nil),
			key:           "key",
			existingValue: big.NewInt(5),
			value:         big.NewInt(4),
			expectedValue: big.NewInt(5),
		},
		{
			name:          "not found",
			store:         newTestBaseStore(t, pbsubstreams.Module_KindStore_UPDATE_POLICY_UNSET, "", nil),
			key:           "key",
			existingValue: nil,
			value:         big.NewInt(4),
			expectedValue: big.NewInt(4),
		},
	}

	initTestStore := func(b *baseStore, key string, value *big.Int) {
		b.kv = map[string][]byte{}
		if value != nil {
			b.kv[key] = []byte(value.String())
		}
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			initTestStore(test.store, test.key, test.existingValue) // b.kv = []{}

			test.store.SetMaxBigInt(0, test.key, test.value)
			actual, found := test.store.GetAt(0, test.key)
			if !found {
				t.Errorf("value not found")
			}

			actualInt, _ := new(big.Int).SetString(string(actual), 10)

			assert.Equal(t, 0, actualInt.Cmp(test.expectedValue))
		})
	}
}

func TestStoreSetMaxInt64(t *testing.T) {
	int64ptr := func(i int64) *int64 {
		var p *int64
		p = new(int64)
		*p = i
		return p
	}

	tests := []struct {
		name          string
		store         *baseStore
		key           string
		existingValue *int64
		value         int64
		expectedValue int64
	}{
		{
			name:          "found less",
			store:         newTestBaseStore(t, pbsubstreams.Module_KindStore_UPDATE_POLICY_UNSET, "", nil),
			key:           "key",
			existingValue: int64ptr(3),
			value:         4,
			expectedValue: 4,
		},
		{
			name:          "found greater",
			store:         newTestBaseStore(t, pbsubstreams.Module_KindStore_UPDATE_POLICY_UNSET, "", nil),
			key:           "key",
			existingValue: int64ptr(5),
			value:         4,
			expectedValue: 5,
		},
		{
			name:          "not found",
			store:         newTestBaseStore(t, pbsubstreams.Module_KindStore_UPDATE_POLICY_UNSET, "", nil),
			key:           "key",
			existingValue: nil,
			value:         4,
			expectedValue: 4,
		},
	}

	initTestStore := func(b *baseStore, key string, value *int64) {
		b.kv = map[string][]byte{}
		if value != nil {
			b.kv[key] = []byte(fmt.Sprintf("%d", *value))
		}
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			initTestStore(test.store, test.key, test.existingValue)

			test.store.SetMaxInt64(0, test.key, test.value)
			actual, found := test.store.GetAt(0, test.key)
			if !found {
				t.Errorf("value not found")
			}

			actualInt, err := strconv.ParseInt(string(actual), 10, 64)
			assert.NoError(t, err)

			assert.Equal(t, test.expectedValue, actualInt)
		})
	}
}

func TestStoreSetMaxFloat64(t *testing.T) {
	float64ptr := func(i float64) *float64 {
		var p *float64
		p = new(float64)
		*p = i
		return p
	}

	tests := []struct {
		name          string
		store         *baseStore
		key           string
		existingValue *float64
		value         float64
		expectedValue float64
	}{
		{
			name:          "found less",
			store:         newTestBaseStore(t, pbsubstreams.Module_KindStore_UPDATE_POLICY_UNSET, "", nil),
			key:           "key",
			existingValue: float64ptr(3.0),
			value:         4.0,
			expectedValue: 4.0,
		},
		{
			name:          "found greater",
			store:         newTestBaseStore(t, pbsubstreams.Module_KindStore_UPDATE_POLICY_UNSET, "", nil),
			key:           "key",
			existingValue: float64ptr(5.0),
			value:         4.0,
			expectedValue: 5.0,
		},
		{
			name:          "not found",
			store:         newTestBaseStore(t, pbsubstreams.Module_KindStore_UPDATE_POLICY_UNSET, "", nil),
			key:           "key",
			existingValue: nil,
			value:         4.0,
			expectedValue: 4.0,
		},
	}

	initTestStore := func(b *baseStore, key string, value *float64) {
		b.kv = map[string][]byte{}
		if value != nil {
			b.kv[key] = []byte(strconv.FormatFloat(*value, 'g', 100, 64))
		}
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			initTestStore(test.store, test.key, test.existingValue)

			test.store.SetMaxFloat64(0, test.key, test.value)
			actual, found := test.store.GetAt(0, test.key)
			if !found {
				t.Errorf("value not found")
			}

			actualInt, err := strconv.ParseFloat(string(actual), 64)
			assert.NoError(t, err)

			assert.Equal(t, test.expectedValue, actualInt)
		})
	}
}

func TestStoreSetMaxBigFloat(t *testing.T) {
	tests := []struct {
		name          string
		store         *baseStore
		key           string
		existingValue *big.Float
		value         *big.Float
		expectedValue *big.Float
	}{
		{
			name:          "found less",
			store:         newTestBaseStore(t, pbsubstreams.Module_KindStore_UPDATE_POLICY_UNSET, "", nil),
			key:           "key",
			existingValue: big.NewFloat(3),
			value:         big.NewFloat(4),
			expectedValue: big.NewFloat(4),
		},
		{
			name:          "found greater",
			store:         newTestBaseStore(t, pbsubstreams.Module_KindStore_UPDATE_POLICY_UNSET, "", nil),
			key:           "key",
			existingValue: big.NewFloat(5),
			value:         big.NewFloat(4),
			expectedValue: big.NewFloat(5),
		},
		{
			name:          "not found",
			store:         newTestBaseStore(t, pbsubstreams.Module_KindStore_UPDATE_POLICY_UNSET, "", nil),
			key:           "key",
			existingValue: nil,
			value:         big.NewFloat(4),
			expectedValue: big.NewFloat(4),
		},
	}

	initTestStore := func(b *baseStore, key string, value *big.Float) {
		b.kv = map[string][]byte{}
		if value != nil {
			b.kv[key] = []byte(value.Text('g', -1))
		}
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			initTestStore(test.store, test.key, test.existingValue)

			test.store.SetMaxBigDecimal(0, test.key, test.value)
			actual, found := test.store.GetAt(0, test.key)
			if !found {
				t.Errorf("value not found")
			}

			actualInt, _, err := big.ParseFloat(string(actual), 10, 100, big.ToNearestEven)
			assert.NoError(t, err)

			assert.Equal(t, 0, actualInt.Cmp(test.expectedValue))
		})
	}
}
