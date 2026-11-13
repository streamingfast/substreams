package app

import (
	"testing"

	"github.com/streamingfast/dstore"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

func TestHubMaxKeptFinalBlocks(t *testing.T) {
	assert.Equal(t, 2000, HubMaxKeptFinalBlocks(0))
	assert.Equal(t, 2000, HubMaxKeptFinalBlocks(100))
	assert.Equal(t, 20000, HubMaxKeptFinalBlocks(1000))
}

func TestNewLiveHubRequiresMergedBlocksStore(t *testing.T) {
	_, err := NewLiveHub(LiveHubConfig{OneBlocksStore: dstore.NewMockStore(nil), Logger: zap.NewNop()})
	assert.Error(t, err)
}
