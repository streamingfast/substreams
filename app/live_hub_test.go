package app

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHubKeepFinalBlocks(t *testing.T) {
	assert.Equal(t, 200, HubKeepFinalBlocks(1))
	assert.Equal(t, 200, HubKeepFinalBlocks(100))
	assert.Equal(t, 400, HubKeepFinalBlocks(200))
	assert.Equal(t, 2000, HubKeepFinalBlocks(1000))
}
