package info

import (
	"github.com/charmbracelet/bubbles/key"

	"github.com/streamingfast/substreams/tui2/common"
	"github.com/streamingfast/substreams/tui2/keymap"
)

func (d *Info) ShortHelp() []key.Binding {
	return []key.Binding{
		keymap.UpDown,
		keymap.MainNavigation,
		keymap.RestartStream,
		keymap.Help,
		keymap.Quit,
	}
}

func (d *Info) FullHelp() [][]key.Binding {
	return common.ShortToFullHelp(d)
}
