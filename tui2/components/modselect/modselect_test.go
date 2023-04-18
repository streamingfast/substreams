package modselect

import (
	"github.com/streamingfast/substreams/manifest"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestGetColumns(t *testing.T) {
	modSelect := newTestModSelect(manifest.NewSimpleTestModules())
	g, err := modSelect.SetColumns()
	assert.NoError(t, err)

	want := [][]int{{4}, {1, 3, 6}, {0, 2}}
	assert.Equal(t, want, g)
}

func TestModSelect_GetRenderedColumns(t *testing.T) {
	modSelect := newTestModSelect(manifest.NewSimpleTestModules())

	modSelect.Selected = 3
	modSelect.Highlighted = 6
	modSelect.Seen = map[string]bool{
		"A": true,
		"B": false,
		"C": true,
		"D": true,
		"E": true,
		"F": true,
		"X": true,
	}

	r, err := modSelect.GetRenderedColumns()
	assert.NoError(t, err)

	_ = r
}

func TestColumnRender(t *testing.T) {

}
