package output

import (
	"github.com/charmbracelet/bubbles/key"

	"github.com/streamingfast/substreams/tui2/keymap"
)

func (o *Output) ShortHelp() []key.Binding {
	return []key.Binding{
		keymap.PrevNextModule,
		keymap.PrevNextBlock,
		keymap.Search,
		keymap.ModuleSearch,
		keymap.Help,
		keymap.ToggleLogs,
		keymap.GoToBlock,
		keymap.ModGraphView,
	}
}

func (o *Output) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{
			keymap.PrevNextModule,
			keymap.ToggleLogs,
			keymap.ToggleBytesFormat,
		},
		{
			keymap.PrevNextBlock,
			keymap.UpDown,
		},
		{
			keymap.Search,
			keymap.PrevNextSearchResult,
			keymap.PrevNextMatchedBlock,
		},
		{
			keymap.ModuleSearch,
			keymap.RestartStream,
		},
		{
			keymap.Help,
		},
	}
}
