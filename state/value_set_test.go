package state

import (
	"testing"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/stretchr/testify/assert"
)

func TestValueAppend(t *testing.T) {
	s := mustNewBuilder(t, "b", 0, "hash", pbsubstreams.Module_KindStore_UPDATE_POLICY_UNSET, "", nil)

	s.Append(0, "key", []byte{0x00, 0x01, 0x02})
	s.Append(1, "key", []byte{0x03, 0x04, 0x05})
	s.Append(1, "key", []byte{0x06})
	res, found := s.GetLast("key")
	assert.True(t, found)
	assert.Equal(t, []byte{0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06}, res)
}
