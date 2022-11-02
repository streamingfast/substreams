package reqctx

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestRequestDetails_SkipSnapshotSave(t *testing.T) {
	outMods := map[string]bool{"A": true}
	assert.True(t, (&RequestDetails{IsSubRequest: true, IsOutputModule: outMods}).SkipSnapshotSave("B"))
	assert.False(t, (&RequestDetails{IsSubRequest: true, IsOutputModule: outMods}).SkipSnapshotSave("A"))
	assert.False(t, (&RequestDetails{IsSubRequest: false, IsOutputModule: outMods}).SkipSnapshotSave("B"))
	assert.False(t, (&RequestDetails{IsSubRequest: false, IsOutputModule: outMods}).SkipSnapshotSave("A"))
}
