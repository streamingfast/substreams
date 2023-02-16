package output

import (
	"github.com/charmbracelet/bubbles/viewport"
	"github.com/jhump/protoreflect/desc"

	"github.com/charmbracelet/bubbles/key"

	"github.com/streamingfast/substreams/tui2/components/blockselect"

	"github.com/streamingfast/substreams/tui2/components/modselect"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/streamingfast/substreams/tui2/common"
)

type Output struct {
	common.Common

	msgDescs map[string]*desc.MessageDescriptor

	moduleSelector    *modselect.ModSelect
	blockSelector     *blockselect.BlockSelect
	outputView        viewport.Model
	lastOutputContent *pbsubstreams.ModuleOutput

	lowBlock  uint64
	highBlock uint64

	blocksPerModule map[string][]uint64
	payloads        map[string]map[uint64]*pbsubstreams.ModuleOutput
	blockIDs        map[uint64]string

	activeModule string
	activeBlock  uint64
}

func New(c common.Common, msgDescs map[string]*desc.MessageDescriptor) *Output {
	return &Output{
		Common:          c,
		msgDescs:        msgDescs,
		blocksPerModule: make(map[string][]uint64),
		payloads:        make(map[string]map[uint64]*pbsubstreams.ModuleOutput),
		blockIDs:        make(map[uint64]string),
		moduleSelector:  modselect.New(c),
		blockSelector:   blockselect.New(c),
		outputView:      viewport.New(24, 80),
	}
}

func (o *Output) Init() tea.Cmd {
	//o.outputView.HighPerformanceRendering = true
	return tea.Batch(
		o.moduleSelector.Init(),
		o.blockSelector.Init(),
	)
}

func (o *Output) SetSize(w, h int) {
	o.Common.SetSize(w, h)
	o.moduleSelector.SetSize(w, 2)
	o.blockSelector.SetSize(w, 5)
	o.outputView.Width = w
	// header, block info in output
	o.outputView.Height = h - 11
	outputViewTopBorder := 1
	o.outputView.Height = h - o.moduleSelector.Height - o.blockSelector.Height - outputViewTopBorder
}

func (o *Output) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// WARN: this will not be so pretty for the reversible segment, as we're
	// flattening the block IDs into numbers...
	// Probably fine for now, as we're debugging the history.
	var cmds []tea.Cmd
	switch msg := msg.(type) {
	case *pbsubstreams.BlockScopedData:
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
			o.setViewportContent()
		}
	case modselect.ModuleSelectedMsg:
		o.activeModule = string(msg)
		o.blockSelector.SetAvailableBlocks(o.blocksPerModule[o.activeModule])
		o.setViewportContent()
	case blockselect.BlockSelectedMsg:
		o.activeBlock = uint64(msg)
		o.setViewportContent()
	case tea.KeyMsg:
		_, cmd := o.moduleSelector.Update(msg)
		cmds = append(cmds, cmd)
		_, cmd = o.blockSelector.Update(msg)
		cmds = append(cmds, cmd)
		o.outputView, cmd = o.outputView.Update(msg)
		cmds = append(cmds, cmd)
	}
	return o, tea.Batch(cmds...)
}

func (o *Output) setViewportContent() {
	if mod, found := o.payloads[o.activeModule]; found {
		if payload, found := mod[o.activeBlock]; found {
			// Do the decoding once per view, and cache the decoded value if it hasn't changed
			if payload != o.lastOutputContent {
				o.outputView.SetContent(o.renderPayload(payload))
				o.lastOutputContent = payload
			}
		} else {
			o.outputView.SetContent("")
			o.lastOutputContent = nil
		}
	} else {
		o.outputView.SetContent("")
		o.lastOutputContent = nil
	}
}

func (o *Output) View() string {
	return lipgloss.JoinVertical(0,
		o.moduleSelector.View(),
		o.blockSelector.View(),
		"",
		o.outputView.View(),
	)
}

var Styles = struct {
	LogLabel  lipgloss.Style
	LogLine   lipgloss.Style
	ErrorLine lipgloss.Style
}{
	LogLabel:  lipgloss.NewStyle().Foreground(lipgloss.Color("243")),
	LogLine:   lipgloss.NewStyle().Foreground(lipgloss.Color("252")),
	ErrorLine: lipgloss.NewStyle().Foreground(lipgloss.Color("1")),
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
