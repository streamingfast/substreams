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
	}{
		{
			name:           "golden path",
			store:          newTestBaseStore(t, pbsubstreams.Module_KindStore_UPDATE_POLICY_UNSET, "", nil, &BinaryMarshaller{}),
			key:            "key",
			values:         [][]byte{{0x00, 0x01, 0x02}, {0x03, 0x04, 0x05}, {0x06}},
			expectedValues: []byte{0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			for _, v := range test.values {
				test.store.Append(0, test.key, v)
			}
			res, found := test.store.GetLast(test.key)
			assert.True(t, found)
			assert.Equal(t, test.expectedValues, res)
		})
	}

}
