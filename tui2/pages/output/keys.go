package output

import (
	"github.com/charmbracelet/bubbles/key"

	"github.com/streamingfast/substreams/tui2/keymap"
)

func (o *Output) ShortHelp() []key.Binding {
	return []key.Binding{
		keymap.PrevNextBlock,
		keymap.PrevNextModule,
		keymap.Search,
		keymap.ModuleSearch,
		keymap.ToggleLogs,
		keymap.Build,
		keymap.UpDown,
		keymap.Help,
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
			keymap.FirstLastBlock,
			keymap.UpDown,
			keymap.GoToBlock,
		},
		{
			keymap.Search,
			keymap.PrevNextSearchResult,
			keymap.PrevNextMatchedBlock,
		},
		{
			keymap.ModuleSearch,
			keymap.ModGraphView,
			keymap.RestartStream,
		},
		{
			keymap.Build,
		},
		{
			keymap.Help,
			keymap.MainNavigation,
			keymap.Quit,
		},
	}
}
