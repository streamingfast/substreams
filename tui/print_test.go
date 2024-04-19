package tui

import (
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/stretchr/testify/assert"
	"testing"
)

func Test_printUndoJSON(t *testing.T) {
	assert.Equal(t,
		`{"undo_until":{"num":1,"id":"1","next_cursor":"aa"}}`,
		formatUndoJSON(&pbsubstreams.BlockRef{
			Id:     "1",
			Number: 1,
		}, "aa"),
	)
}
