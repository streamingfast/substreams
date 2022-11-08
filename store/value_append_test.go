package store

import (
	"testing"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/stretchr/testify/assert"
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
			name:           "append over limit",
			store:          newTestBaseStore(t, pbsubstreams.Module_KindStore_UPDATE_POLICY_APPEND, "", nil),
			key:            "key",
			values:         [][]byte{{0x00, 0x01, 0x02}, {0x03, 0x04, 0x05}, {0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10}},
			expectedValues: []byte{0x00, 0x01, 0x02, 0x03, 0x04, 0x05},
			expectedError:  true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var err error
			for _, v := range test.values {
				err = test.store.Append(0, test.key, v)
				if !test.expectedError {
					assert.NoError(t, err)
				}
				if err != nil {
					break
				}
			}
			if test.expectedError {
				assert.Error(t, err)
			}

			res, found := test.store.GetLast(test.key)
			assert.True(t, found)
			assert.Equal(t, test.expectedValues, res)
		})
	}

}
