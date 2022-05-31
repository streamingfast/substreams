package block

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRange_Split(t *testing.T) {
	og := &Range{
		StartBlock:        706,
		ExclusiveEndBlock: 1250,
	}

	expected := []*Range{
		{706, 900},
		{900, 1100},
		{1100, 1250},
	}

	actual := og.Split(200)

	require.Equal(t, expected, actual)
}

func TestRange_Split2(t *testing.T) {
	og := &Range{
		StartBlock:        6811700,
		ExclusiveEndBlock: 6811900,
	}

	actual := og.Split(100)

	require.Equal(t, []*Range{
		{6811700, 6811800},
		{6811800, 6811900},
	}, actual)
}
