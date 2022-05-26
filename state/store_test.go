package state

import (
	"testing"

	"github.com/streamingfast/substreams/block"
	"github.com/stretchr/testify/require"
)

func TestFileName(t *testing.T) {
	prefix := FullStateFilePrefix(10000)
	require.Equal(t, "0000010000", prefix)

	stateFileName := FullStateFileName(&block.Range{StartBlock: 100, ExclusiveEndBlock: 10000}, 100)
	require.Equal(t, "0000010000-0000000100.kv", stateFileName)

	partialFileName := PartialFileName(&block.Range{StartBlock: 10000, ExclusiveEndBlock: 20000})
	require.Equal(t, "0000020000-0000010000.partial", partialFileName)
}
