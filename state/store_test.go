package state

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestFileName(t *testing.T) {
	prefix := StateFilePrefix("test", 10000)
	require.Equal(t, "test-0000010000", prefix)

	stateFileName := StateFileName("test", 100, 10000)
	require.Equal(t, "test-0000010000-0000000100.kv", stateFileName)

	partialFileName := PartialFileName("test", 10000, 20000)
	require.Equal(t, "test-0000020000-0000010000.partial", partialFileName)
}
