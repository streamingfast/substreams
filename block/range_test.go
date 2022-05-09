package block

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestRange_Split(t *testing.T) {
	og := &Range{
		StartBlock:        100,
		ExclusiveEndBlock: 30_000,
	}

	expected := []*Range{
		{100, 10_100},
		{10_100, 20_100},
		{20_100, 30_000},
	}

	actual := og.Split(10_000)

	require.Equal(t, expected, actual)
}
