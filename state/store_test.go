package state

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFileName(t *testing.T) {
	prefix := StateFilePrefix(10000)
	require.Equal(t, "0000010000", prefix)

	stateFileName := StateFileName(100, 10000)
	require.Equal(t, "0000010000-0000000100.kv", stateFileName)

	partialFileName := PartialFileName(10000, 20000)
	require.Equal(t, "0000020000-0000010000.partial", partialFileName)
}
