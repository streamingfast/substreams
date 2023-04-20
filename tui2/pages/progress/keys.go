package progress

import (
	"github.com/charmbracelet/bubbles/key"

	"github.com/streamingfast/substreams/tui2/keymap"
)

func (p *Progress) ShortHelp() []key.Binding {
	return []key.Binding{
		keymap.UpDown,
		keymap.ToggleProgressDisplayMode,
		keymap.RestartStream,
		keymap.Help,
		keymap.Quit,
	}
}

func (p *Progress) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{
			keymap.UpDown,
			keymap.UpDownPage,
		},
		{
			keymap.ToggleProgressDisplayMode,
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
