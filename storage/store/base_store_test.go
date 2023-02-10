package store

import (
	"testing"

	"github.com/streamingfast/substreams/block"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"

	"github.com/stretchr/testify/require"
	"github.com/test-go/testify/assert"
)

func TestStore(t *testing.T) {
	s := newTestBaseStore(t, pbsubstreams.Module_KindStore_UPDATE_POLICY_UNSET, "", nil)

	s.Set(0, "1", "val1")
	s.Set(1, "1", "val2")
	s.Set(3, "1", "val3")
	s.Reset()
	s.Set(0, "1", "val4")
	s.Set(1, "1", "val5")
	s.Set(3, "1", "val6")
	s.Set(5, "1", "val7")
	s.Set(6, "3", "foo")
	s.Set(7, "3", "bar")
	s.DeletePrefix(8, "3")
	s.Set(9, "4", "baz")

	var found bool

	val, found := s.GetFirst("1")
	assert.Equal(t, "val3", string(val))
	assert.True(t, found)

	found = s.HasFirst("1")
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

	val, found = s.GetAt(5, "1")
	assert.Equal(t, "val7", string(val))
	assert.True(t, found)

	val, found = s.GetLast("1")
	assert.Equal(t, "val7", string(val))
	assert.True(t, found)

	found = s.HasLast("1")
	assert.True(t, found)

	for i := 0; i < 5; i++ {
		found = s.HasAt(uint64(i), "1")
		assert.True(t, found)
	}

	found = s.HasAt(5, "0")
	assert.False(t, found)

	found = s.HasAt(7, "3")
	assert.True(t, found)

	found = s.HasAt(9, "3")
	assert.False(t, found)
}

func TestFileName(t *testing.T) {
	prefix := fullStateFilePrefix(10000)
	require.Equal(t, "0000010000", prefix)

	stateFileName := fullStateFileName(&block.Range{StartBlock: 100, ExclusiveEndBlock: 10000})
	require.Equal(t, "0000010000-0000000100.kv", stateFileName)

	partialFileName := partialFileName(&block.Range{StartBlock: 10000, ExclusiveEndBlock: 20000})
	require.Equal(t, "0000020000-0000010000.partial", partialFileName)
}
