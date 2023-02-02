package output

import (
	"fmt"

	"github.com/charmbracelet/bubbles/key"

	"github.com/streamingfast/substreams/tui2/components/blockselect"

	"github.com/streamingfast/substreams/tui2/components/modselect"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/streamingfast/substreams/tui2/common"
	"github.com/streamingfast/substreams/tui2/stream"
)

type Output struct {
	common.Common

	moduleSelector *modselect.ModSelect
	blockSelector  *blockselect.BlockSelect

	lowBlock  uint64
	highBlock uint64

	blocksPerModule map[string][]uint64
	payloads        map[string]map[uint64]*pbsubstreams.ModuleOutput
	blockIDs        map[uint64]string

	activeModule string
	activeBlock  uint64
}

func New(c common.Common) *Output {
	return &Output{
		Common:          c,
		blocksPerModule: make(map[string][]uint64),
		payloads:        make(map[string]map[uint64]*pbsubstreams.ModuleOutput),
		blockIDs:        make(map[uint64]string),
		moduleSelector:  modselect.New(c),
		blockSelector:   blockselect.New(c),
	}
}

func (o *Output) Init() tea.Cmd {
	return tea.Batch(
		o.moduleSelector.Init(),
		o.blockSelector.Init(),
	)
}

func (o *Output) SetSize(w, h int) {
	o.Common.SetSize(w, h)
	o.moduleSelector.SetSize(w, 2)
	o.blockSelector.SetSize(w, 2)
}

func (o *Output) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// WARN: this will not be so pretty for the reversible segment, as we're
	// flattening the block IDs into numbers...
	// Probably fine for now, as we're debugging the history.
	var cmds []tea.Cmd
	switch msg := msg.(type) {
	case stream.ResponseDataMsg:
		blockNum := msg.Clock.Number

		if o.lowBlock == 0 {
			o.lowBlock = blockNum
		}
		if o.highBlock < blockNum {
			o.highBlock = blockNum
		}
		o.blockSelector.StretchBounds(o.lowBlock, o.highBlock)

		o.blockIDs[msg.Clock.Number] = msg.Clock.Id
		for _, output := range msg.Outputs {
			if isEmptyModuleOutput(output) {
				continue
			}

			modName := output.Name

			modulePayloads, found := o.payloads[modName]
			if !found {
				o.moduleSelector.AddModule(modName)
				if o.activeModule == "" {
					o.activeModule = modName
				}
				modulePayloads = make(map[uint64]*pbsubstreams.ModuleOutput)
			}
			if _, found := modulePayloads[blockNum]; !found {
				if o.activeBlock == 0 {
					o.activeBlock = blockNum
				}
				o.blocksPerModule[modName] = append(o.blocksPerModule[modName], blockNum)
				if modName == o.activeModule {
					o.blockSelector.SetAvailableBlocks(o.blocksPerModule[modName])
				}
			}
			modulePayloads[blockNum] = output
			o.payloads[modName] = modulePayloads
		}
	case modselect.ModuleSelectedMsg:
		o.activeModule = string(msg)
		o.blockSelector.SetAvailableBlocks(o.blocksPerModule[o.activeModule])
	case blockselect.BlockSelectedMsg:
		o.activeBlock = uint64(msg)
	case tea.KeyMsg:
		_, cmd := o.moduleSelector.Update(msg)
		cmds = append(cmds, cmd)
		_, cmd = o.blockSelector.Update(msg)
		cmds = append(cmds, cmd)
	}
	return o, tea.Batch(cmds...)
}

func (o *Output) View() string {
	return lipgloss.JoinVertical(0,
		"",
		fmt.Sprintf("Active module: %s", o.activeModule),
		fmt.Sprintf("Active block: %d", o.activeBlock),
		fmt.Sprintf("Block range: %d - %d (total: %d)", o.lowBlock, o.highBlock, o.highBlock-o.lowBlock),
		o.moduleSelector.View(),
		o.blockSelector.View(),
	)
}

func (o *Output) ShortHelp() []key.Binding {
	return []key.Binding{
		key.NewBinding(
			key.WithKeys("u", "i"),
			key.WithHelp("u/i", "Nav. modules"),
		),
		key.NewBinding(
			key.WithKeys("o", "p"),
			key.WithHelp("o/p", "Nav. blocks"),
		),
		key.NewBinding(
			key.WithKeys("up", "k", "down", "j"),
			key.WithHelp("↑/k/↓/j", "up/down"),
		),
	}
}

func (o *Output) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		o.ShortHelp(),
	}
}
