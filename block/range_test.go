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
