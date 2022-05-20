package block

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRange_Split(t *testing.T) {
	og := &Range{
		StartBlock:        706,
		ExclusiveEndBlock: 1000,
	}

	expected := []*Range{
		{706, 800},
		{800, 900},
		{900, 1000},
	}

	actual := og.Split(100)

	require.Equal(t, expected, actual)
}
