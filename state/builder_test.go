package state

import (
	"testing"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"

	"github.com/stretchr/testify/assert"
)

func TestStateBuilder(t *testing.T) {
	s := NewBuilder("b", 0, pbsubstreams.Module_KindStore_UPDATE_POLICY_UNSET, "", nil)

	s.Set(0, "1", "val1")
	s.Set(1, "1", "val2")
	s.Set(3, "1", "val3")
	s.Flush()
	s.Set(0, "1", "val4")
	s.Set(1, "1", "val5")
	s.Set(3, "1", "val6")
	s.Del(4, "1")
	s.Set(5, "1", "val7")

	val, found := s.GetFirst("1")
	assert.Equal(t, "val3", string(val))
	assert.True(t, found)

	val, found = s.GetAt(0, "1")
	assert.Equal(t, "val4", string(val))
	assert.True(t, found)

	val, found = s.GetAt(1, "1")
	assert.Equal(t, "val5", string(val))
	assert.True(t, found)

	val, found = s.GetAt(3, "1")
	assert.Equal(t, "val6", string(val))
	assert.True(t, found)

	val, found = s.GetAt(4, "1")
	assert.Nil(t, val)
	assert.False(t, found)

	val, found = s.GetAt(5, "1")
	assert.Equal(t, "val7", string(val))
	assert.True(t, found)

	val, found = s.GetLast("1")
	assert.Equal(t, "val7", string(val))
	assert.True(t, found)
}
