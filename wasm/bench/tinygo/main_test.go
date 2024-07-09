package main

import (
	"testing"

	"github.com/streamingfast/tinygo-test/pb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMapTest(t *testing.T) {
	store := PrepareStore("myfixtures.json")
	_ = store
	res, err := map_test(&pb.Block{})
	require.NoError(t, err)
	assert.Equal(t, 123, res.Number)
}
