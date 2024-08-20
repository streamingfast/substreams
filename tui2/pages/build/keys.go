package build

import (
	"github.com/charmbracelet/bubbles/key"

	"github.com/streamingfast/substreams/tui2/common"
	"github.com/streamingfast/substreams/tui2/keymap"
)

func (d *Build) ShortHelp() []key.Binding {
	return []key.Binding{
		keymap.Build,
		keymap.UpDown,
		keymap.MainNavigation,
		keymap.Help,
		keymap.Quit,
	}
}

func (d *Build) FullHelp() [][]key.Binding {
	return common.ShortToFullHelp(d)
}
