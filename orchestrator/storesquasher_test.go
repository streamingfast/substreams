package orchestrator

import (
	"testing"

	"github.com/streamingfast/substreams/block"
	"github.com/stretchr/testify/assert"
)

func TestShouldSaveFullKV(t *testing.T) {
	squasher := func(interval uint64) *StoreSquasher {
		return &StoreSquasher{storeSaveInterval: interval}
	}
	assert.True(t, squasher(10).shouldSaveFullKV(10, block.ParseRange("10-20")))
}
