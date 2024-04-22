package store

import (
	"testing"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValueAppend(t *testing.T) {
	tests := []struct {
		name           string
		store          *baseStore
		key            string
		values         [][]byte
		expectedValues []byte
		expectedError  bool
	}{
		{
			name:           "golden path",
			store:          newTestBaseStore(t, pbsubstreams.Module_KindStore_UPDATE_POLICY_APPEND, "", nil),
			key:            "key",
			values:         [][]byte{{0x00, 0x01, 0x02}, {0x03, 0x04, 0x05}, {0x06}},
			expectedValues: []byte{0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06},
			expectedError:  false,
		},
		{
			name:           "append over limit (won't make it in the store)",
			store:          newTestBaseStore(t, pbsubstreams.Module_KindStore_UPDATE_POLICY_APPEND, "", nil),
			key:            "key",
			values:         [][]byte{{0x00, 0x01, 0x02}, {0x03, 0x04, 0x05}, {0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10}},
			expectedValues: []byte{0x00, 0x01, 0x02, 0x03, 0x04, 0x05},
			expectedError:  true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			for _, v := range test.values {
				test.store.Append(0, test.key, v)
			}

			err := test.store.Flush()
			if test.expectedError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			res, found := test.store.GetLast(test.key)
			assert.True(t, found)
			assert.Equal(t, test.expectedValues, res)
		})
	}

}
