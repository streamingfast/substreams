package common

import (
	"github.com/streamingfast/substreams/tui2/keymap"
	"github.com/streamingfast/substreams/tui2/styles"
)

// Common is a struct all components should embed.
type Common struct {
	Styles *styles.Styles
	KeyMap *keymap.KeyMap
	Width  int
	Height int
}

// SetSize sets the width and height of the common struct.
func (c *Common) SetSize(width, height int) {
	c.Width = width
	c.Height = height
}
