package reqctx

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRequestDetails_SkipSnapshotSave(t *testing.T) {
	assert.True(t, (&RequestDetails{IsSubRequest: true, OutputModule: "A"}).SkipSnapshotSave("B"))
	assert.False(t, (&RequestDetails{IsSubRequest: true, OutputModule: "A"}).SkipSnapshotSave("A"))
	assert.False(t, (&RequestDetails{IsSubRequest: false, OutputModule: "A"}).SkipSnapshotSave("B"))
	assert.False(t, (&RequestDetails{IsSubRequest: false, OutputModule: "A"}).SkipSnapshotSave("A"))
}
