package request

import (
	"github.com/charmbracelet/bubbles/key"

	"github.com/streamingfast/substreams/tui2/keymap"
)

func (r *Request) ShortHelp() []key.Binding {
	return []key.Binding{
		keymap.MainNavigation,
		keymap.UpDown,
		keymap.RestartStream,
		keymap.Help,
		keymap.Quit,
	}
}
func (r *Request) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{
			keymap.MainNavigation,
		},
		{
			keymap.UpDown,
			keymap.UpDownPage,
		},
		{
			keymap.RestartStream,
		},
		{
			keymap.Help,
		},
		{
			keymap.Quit,
		},
	}
}
