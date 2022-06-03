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
		{706, 800},
		{800, 1000},
		{1000, 1200},
		{1200, 1250},
	}

	actual := og.Split(200)

	require.Equal(t, expected, actual)
}
