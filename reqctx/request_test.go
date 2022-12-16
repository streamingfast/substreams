package reqctx

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestRequestDetails_SkipSnapshotSave(t *testing.T) {
	outModFunc := func(name string) bool { return name == "A" }
	assert.True(t, (&RequestDetails{IsSubRequest: true, IsOutputModule: outModFunc}).SkipSnapshotSave("B"))
	assert.False(t, (&RequestDetails{IsSubRequest: true, IsOutputModule: outModFunc}).SkipSnapshotSave("A"))
	assert.False(t, (&RequestDetails{IsSubRequest: false, IsOutputModule: outModFunc}).SkipSnapshotSave("B"))
	assert.False(t, (&RequestDetails{IsSubRequest: false, IsOutputModule: outModFunc}).SkipSnapshotSave("A"))
}
