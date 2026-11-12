package app

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHubMaxKeptFinalBlocks(t *testing.T) {
	assert.Equal(t, 2000, HubMaxKeptFinalBlocks(0))
	assert.Equal(t, 2000, HubMaxKeptFinalBlocks(100))
	assert.Equal(t, 20000, HubMaxKeptFinalBlocks(1000))
}
