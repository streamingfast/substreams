package state

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStateBuilder(t *testing.T) {
	s := New("", "", nil)
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
	assert.Equal(t, string("val3"), val.String())
	assert.True(t, found)

	val, found = s.GetAt(0, "1")
	assert.Equal(t, string("val4"), val.String())
	assert.True(t, found)

	val, found = s.GetAt(1, "1")
	assert.Equal(t, string("val5"), val.String())
	assert.True(t, found)

	val, found = s.GetAt(3, "1")
	assert.Equal(t, string("val6"), val.String())
	assert.True(t, found)

	val, found = s.GetAt(4, "1")
	assert.Nil(t, val.Value)
	assert.False(t, found)

	val, found = s.GetAt(5, "1")
	assert.Equal(t, string("val7"), val.String())
	assert.True(t, found)

	val, found = s.GetLast("1")
	assert.Equal(t, string("val7"), val.String())
	assert.True(t, found)
}
