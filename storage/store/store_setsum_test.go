package store

import (
	"testing"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStoreSetSumInt64(t *testing.T) {
	tests := []struct {
		name                string
		key                 string
		existingValue       []byte
		value               []byte
		expectedGetValue    []byte
		expectedActualValue []byte
	}{
		{
			name:                "sum",
			key:                 "key",
			existingValue:       []byte("sum:3"),
			value:               []byte("sum:7"),
			expectedGetValue:    []byte("10"),
			expectedActualValue: []byte("sum:10"),
		},
		{
			name:                "sum not found",
			key:                 "key",
			existingValue:       nil,
			value:               []byte("sum:7"),
			expectedGetValue:    []byte("7"),
			expectedActualValue: []byte("sum:7"),
		},
		{
			name:                "set",
			key:                 "key",
			existingValue:       []byte("sum:3"),
			value:               []byte("set:7"),
			expectedGetValue:    []byte("7"),
			expectedActualValue: []byte("set:7"),
		},
		{
			name:                "set not found",
			key:                 "key",
			existingValue:       nil,
			value:               []byte("set:7"),
			expectedGetValue:    []byte("7"),
			expectedActualValue: []byte("set:7"),
		},
		{
			name:                "consecutive sets",
			key:                 "key",
			existingValue:       []byte("set:72"),
			value:               []byte("set:7"),
			expectedGetValue:    []byte("7"),
			expectedActualValue: []byte("set:7"),
		},
		{
			name:                "sum after set",
			key:                 "key",
			existingValue:       []byte("set:7"),
			value:               []byte("sum:3"),
			expectedGetValue:    []byte("10"),
			expectedActualValue: []byte("set:10"), // always keep a 'set:' prefix
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			b := newTestBaseStore(t, pbsubstreams.Module_KindStore_UPDATE_POLICY_SET_SUM, "", nil)
			if test.existingValue != nil {
				b.kv[test.key] = test.existingValue
				b.totalSizeBytes += uint64(len(test.key) + len(test.existingValue))
			}

			b.SetSumInt64(0, test.key, test.value)
			require.NoError(t, b.Flush())
			actualGetValue, found := b.GetAt(0, test.key)
			if !found {
				t.Errorf("value not found")
			}

			assert.Equal(t, test.expectedGetValue, actualGetValue)

			actual, found := b.getAt(0, test.key)
			if !found {
				t.Errorf("value not found")
			}

			assert.Equal(t, test.expectedActualValue, actual)
		})
	}

}

func TestStoreSetSumFloat64(t *testing.T) {
	tests := []struct {
		name                string
		key                 string
		existingValue       []byte
		value               []byte
		expectedGetValue    []byte
		expectedActualValue []byte
	}{
		{
			name:                "sum",
			key:                 "key",
			existingValue:       []byte("sum:3.0"),
			value:               []byte("sum:7.0"),
			expectedGetValue:    []byte("10"),
			expectedActualValue: []byte("sum:10"),
		},
		{
			name:                "sum not found",
			key:                 "key",
			existingValue:       nil,
			value:               []byte("sum:7.0"),
			expectedGetValue:    []byte("7.0"),
			expectedActualValue: []byte("sum:7.0"),
		},
		{
			name:                "set",
			key:                 "key",
			existingValue:       []byte("sum:3.0"),
			value:               []byte("set:7.0"),
			expectedGetValue:    []byte("7.0"),
			expectedActualValue: []byte("set:7.0"),
		},
		{
			name:                "set not found",
			key:                 "key",
			existingValue:       nil,
			value:               []byte("set:7.0"),
			expectedGetValue:    []byte("7.0"),
			expectedActualValue: []byte("set:7.0"),
		},
		{
			name:                "consecutive sets",
			key:                 "key",
			existingValue:       []byte("set:72.0"),
			value:               []byte("set:7.0"),
			expectedGetValue:    []byte("7.0"),
			expectedActualValue: []byte("set:7.0"),
		},
		{
			name:                "sum after set",
			key:                 "key",
			existingValue:       []byte("set:7.0"),
			value:               []byte("sum:3.0"),
			expectedGetValue:    []byte("10"),
			expectedActualValue: []byte("set:10"),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			b := newTestBaseStore(t, pbsubstreams.Module_KindStore_UPDATE_POLICY_SET_SUM, "", nil)
			if test.existingValue != nil {
				b.kv[test.key] = test.existingValue
				b.totalSizeBytes += uint64(len(test.key) + len(test.existingValue))
			}

			b.SetSumFloat64(0, test.key, test.value)
			require.NoError(t, b.Flush())
			actualGetValue, found := b.GetAt(0, test.key)
			if !found {
				t.Errorf("value not found")
			}

			assert.Equal(t, test.expectedGetValue, actualGetValue)

			actual, found := b.getAt(0, test.key)
			if !found {
				t.Errorf("value not found")
			}

			assert.Equal(t, test.expectedActualValue, actual)
		})
	}
}

func TestStoreSetSumBigInt(t *testing.T) {
	tests := []struct {
		name                string
		key                 string
		existingValue       []byte
		value               []byte
		expectedGetValue    []byte
		expectedActualValue []byte
	}{
		{
			name:                "sum",
			key:                 "key",
			existingValue:       []byte("sum:3"),
			value:               []byte("sum:7"),
			expectedGetValue:    []byte("10"),
			expectedActualValue: []byte("sum:10"),
		},
		{
			name:                "sum not found",
			key:                 "key",
			existingValue:       nil,
			value:               []byte("sum:7"),
			expectedGetValue:    []byte("7"),
			expectedActualValue: []byte("sum:7"),
		},
		{
			name:                "set",
			key:                 "key",
			existingValue:       []byte("sum:3"),
			value:               []byte("set:7"),
			expectedGetValue:    []byte("7"),
			expectedActualValue: []byte("set:7"),
		},
		{
			name:                "set not found",
			key:                 "key",
			existingValue:       nil,
			value:               []byte("set:7"),
			expectedGetValue:    []byte("7"),
			expectedActualValue: []byte("set:7"),
		},
		{
			name:                "consecutive sets",
			key:                 "key",
			existingValue:       []byte("set:72"),
			value:               []byte("set:7"),
			expectedGetValue:    []byte("7"),
			expectedActualValue: []byte("set:7"),
		},
		{
			name:                "sum after set",
			key:                 "key",
			existingValue:       []byte("set:7"),
			value:               []byte("sum:3"),
			expectedGetValue:    []byte("10"),
			expectedActualValue: []byte("set:10"),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			b := newTestBaseStore(t, pbsubstreams.Module_KindStore_UPDATE_POLICY_SET_SUM, "", nil)
			if test.existingValue != nil {
				b.kv[test.key] = test.existingValue
				b.totalSizeBytes += uint64(len(test.key) + len(test.existingValue))
			}

			b.SetSumBigInt(0, test.key, test.value)
			require.NoError(t, b.Flush())
			actualGetValue, found := b.GetAt(0, test.key)
			if !found {
				t.Errorf("value not found")
			}

			assert.Equal(t, test.expectedGetValue, actualGetValue)

			actual, found := b.getAt(0, test.key)
			if !found {
				t.Errorf("value not found")
			}

			assert.Equal(t, test.expectedActualValue, actual)
		})
	}
}

func TestStoreSetSumBigDecimal(t *testing.T) {
	tests := []struct {
		name                string
		key                 string
		existingValue       []byte
		value               []byte
		expectedGetValue    []byte
		expectedActualValue []byte
	}{
		{
			name:                "sum",
			key:                 "key",
			existingValue:       []byte("sum:3.0"),
			value:               []byte("sum:7.0"),
			expectedGetValue:    []byte("10"),
			expectedActualValue: []byte("sum:10"),
		},
		{
			name:                "sum not found",
			key:                 "key",
			existingValue:       nil,
			value:               []byte("sum:7.0"),
			expectedGetValue:    []byte("7.0"),
			expectedActualValue: []byte("sum:7.0"),
		},
		{
			name:                "set",
			key:                 "key",
			existingValue:       []byte("sum:3.0"),
			value:               []byte("set:7.0"),
			expectedGetValue:    []byte("7.0"),
			expectedActualValue: []byte("set:7.0"),
		},
		{
			name:                "set not found",
			key:                 "key",
			existingValue:       nil,
			value:               []byte("set:7.0"),
			expectedGetValue:    []byte("7.0"),
			expectedActualValue: []byte("set:7.0"),
		},
		{
			name:                "consecutive sets",
			key:                 "key",
			existingValue:       []byte("set:72.0"),
			value:               []byte("set:7.0"),
			expectedGetValue:    []byte("7.0"),
			expectedActualValue: []byte("set:7.0"),
		},
		{
			name:                "sum after set",
			key:                 "key",
			existingValue:       []byte("set:7.0"),
			value:               []byte("sum:3.0"),
			expectedGetValue:    []byte("10"),
			expectedActualValue: []byte("set:10"), // always keep a 'set:' prefix
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			b := newTestBaseStore(t, pbsubstreams.Module_KindStore_UPDATE_POLICY_SET_SUM, "", nil)
			if test.existingValue != nil {
				b.kv[test.key] = test.existingValue
				b.totalSizeBytes += uint64(len(test.key) + len(test.existingValue))
			}

			b.SetSumBigDecimal(0, test.key, test.value)
			require.NoError(t, b.Flush())
			actualGetValue, found := b.GetAt(0, test.key)
			if !found {
				t.Errorf("value not found")
			}

			assert.Equal(t, test.expectedGetValue, actualGetValue)

			actual, found := b.getAt(0, test.key)
			if !found {
				t.Errorf("value not found")
			}

			assert.Equal(t, test.expectedActualValue, actual)
		})
	}
}
