package docs

import (
	"github.com/charmbracelet/bubbles/key"

	"github.com/streamingfast/substreams/tui2/common"
	"github.com/streamingfast/substreams/tui2/keymap"
)

func (d *Docs) ShortHelp() []key.Binding {
	return []key.Binding{
		keymap.UpDown,
		keymap.MainNavigation,
		keymap.Help,
		keymap.Quit,
	}
}

func (d *Docs) FullHelp() [][]key.Binding {
	return common.ShortToFullHelp(d)
}
