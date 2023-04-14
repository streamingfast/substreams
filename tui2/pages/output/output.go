package output

import (
	"github.com/streamingfast/substreams/tui2/components/search"
	"github.com/streamingfast/substreams/tui2/pages/request"

	"github.com/charmbracelet/bubbles/key"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/jhump/protoreflect/dynamic"

	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"

	"github.com/streamingfast/substreams/manifest"
	"github.com/streamingfast/substreams/tui2/common"
	"github.com/streamingfast/substreams/tui2/components/blockselect"
	"github.com/streamingfast/substreams/tui2/components/modselect"
)

type ToggleSearchFocus bool

type Output struct {
	common.Common

	msgDescs       map[string]*manifest.ModuleDescriptor
	messageFactory *dynamic.MessageFactory

	moduleSelector     *modselect.ModSelect
	blockSelector      *blockselect.BlockSelect
	outputView         viewport.Model
	lastDisplayContext *displayContext
	lastOutputContent  interface{}
	//lastRenderedContent string

	lowBlock  uint64
	highBlock uint64

	blocksPerModule     map[string][]uint64
	payloads            map[request.BlockContext]*pbsubstreamsrpc.AnyModuleOutput
	bytesRepresentation dynamic.BytesRepresentation

	blockIDs map[uint64]string

	active            request.BlockContext // module + block
	outputViewYoffset map[request.BlockContext]int
	searchCtx         *search.Search
	searchEnabled     bool
}

func New(c common.Common, manifestPath string) *Output {
	output := &Output{
		Common:              c,
		blocksPerModule:     make(map[string][]uint64),
		payloads:            make(map[request.BlockContext]*pbsubstreamsrpc.AnyModuleOutput),
		blockIDs:            make(map[uint64]string),
		moduleSelector:      modselect.New(c, manifestPath),
		blockSelector:       blockselect.New(c),
		outputView:          viewport.New(24, 80),
		messageFactory:      dynamic.NewMessageFactoryWithDefaults(),
		outputViewYoffset:   map[request.BlockContext]int{},
		searchCtx:           search.New(),
		bytesRepresentation: dynamic.BytesAsHex,
	}
	return output
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
	case request.NewRequestInstance:
		o.msgDescs = msg.MsgDescs
		o.blocksPerModule = make(map[string][]uint64)
		o.payloads = make(map[request.BlockContext]*pbsubstreamsrpc.AnyModuleOutput)
		o.blockIDs = make(map[uint64]string)
		o.outputView.SetContent("")
	case *pbsubstreamsrpc.BlockScopedData:
		blockNum := msg.Clock.Number

		if o.lowBlock == 0 {
			o.lowBlock = blockNum
		}
		if o.highBlock < blockNum {
			o.highBlock = blockNum
		}
		o.blockSelector.StretchBounds(o.lowBlock, o.highBlock)

		o.blockIDs[msg.Clock.Number] = msg.Clock.Id
		for _, output := range msg.AllModuleOutputs() {
			if output.IsEmpty() {
				continue
			}

			modName := output.Name()
			blockCtx := request.BlockContext{
				Module:   modName,
				BlockNum: blockNum,
			}

			if _, found := o.payloads[blockCtx]; !found {
				o.moduleSelector.AddModule(modName)
				if o.active.Module == "" {
					o.active.Module = modName
					o.active.BlockNum = blockNum
				}
				o.blocksPerModule[modName] = append(o.blocksPerModule[modName], blockNum)
				if modName == o.active.Module {
					o.blockSelector.SetAvailableBlocks(o.blocksPerModule[modName])
				}
			}
			o.payloads[blockCtx] = output
			o.setViewportContent()
		}

	case search.ApplySearchQueryMsg:
		o.setViewportContent()
		cmds = append(cmds, o.updateMatchingBlocks())
	case modselect.ModuleSelectedMsg:
		o.active.Module = string(msg)
		o.blockSelector.SetAvailableBlocks(o.blocksPerModule[o.active.Module])
		o.outputView.YOffset = o.outputViewYoffset[o.active]
		o.setViewportContent()
		cmds = append(cmds, o.updateMatchingBlocks())
	case blockselect.BlockSelectedMsg:
		o.active.BlockNum = uint64(msg)
		o.outputView.YOffset = o.outputViewYoffset[o.active]
		o.setViewportContent()
	case tea.KeyMsg:
		o.searchCtx.Update(msg)
		switch msg.String() {
		case "/":
			o.searchEnabled = true
			cmds = append(cmds, o.searchCtx.InitInput())
		case "f":
			o.bytesRepresentation = (o.bytesRepresentation + 1) % 3
		}

		_, cmd := o.moduleSelector.Update(msg)
		cmds = append(cmds, cmd)

		_, cmd = o.blockSelector.Update(msg)
		cmds = append(cmds, cmd)

		o.outputView, cmd = o.outputView.Update(msg)
		cmds = append(cmds, cmd)
		o.outputViewYoffset[o.active] = o.outputView.YOffset

		o.setViewportContent()

	case search.JumpToNextMatchMsg:
		for _, pos := range msg.Positions {
			if pos > o.outputView.YOffset {
				o.outputView.YOffset = pos
				break
			}
		}
	case search.JumpToPreviousMatchMsg:
		for i := len(msg.Positions) - 1; i >= 0; i-- {
			pos := msg.Positions[i]
			if pos < o.outputView.YOffset {
				o.outputView.YOffset = pos
				break
			}
		}
	}
	return o, tea.Batch(cmds...)
}

type displayContext struct {
	blockCtx          request.BlockContext
	searchViewEnabled bool
	searchQuery       string
	payload           *pbsubstreamsrpc.AnyModuleOutput
}

func (o *Output) setViewportContent() {
	dpContext := &displayContext{
		blockCtx:          o.active,
		searchViewEnabled: o.searchEnabled,
		searchQuery:       o.searchCtx.Query,
		payload:           o.payloads[o.active],
	}

	if dpContext != o.lastDisplayContext {
		content := o.renderPayload(dpContext.payload)
		if dpContext.searchViewEnabled {
			var lines int
			var positions []int
			content, lines, positions = applySearchColoring(content, o.searchCtx.Query)
			o.searchCtx.SetMatchCount(lines) //timesFound = lines
			o.searchCtx.SetPositions(positions)
		}
		o.lastDisplayContext = dpContext
		o.outputView.SetContent(content)
	}

}

func (o *Output) View() string {
	var searchLine string
	if o.searchEnabled {
		searchLine = o.searchCtx.View()
	}
	out := lipgloss.JoinVertical(0,
		o.moduleSelector.View(),
		o.blockSelector.View(),
		"",
		o.outputView.View(),
		searchLine,
	)
	return out
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
		key.NewBinding(
			key.WithKeys("/"),
			key.WithHelp("/", "Search"),
		),
		key.NewBinding(
			key.WithKeys("R"),
			key.WithHelp("R", "refresh"),
		),
		key.NewBinding(
			key.WithKeys("f"),
			key.WithHelp("f", "Bytes encoding"),
		),
	}
}

func (o *Output) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		o.ShortHelp(),
	}
}

func (o *Output) updateMatchingBlocks() tea.Cmd {
	if !o.searchEnabled {
		return nil
	}
	return func() tea.Msg {
		matchingBlocks := o.searchAllBlocksForModule(o.active.Module)
		return search.UpdateMatchingBlocks(matchingBlocks)
	}
}

func (o *Output) searchAllBlocksForModule(moduleName string) map[uint64]bool {
	out := make(map[uint64]bool)

	for _, block := range o.blocksPerModule[moduleName] {
		blockCtx := request.BlockContext{
			Module:   moduleName,
			BlockNum: block,
		}
		payload := o.payloads[blockCtx]
		content := o.renderPayload(payload)

		var pos []int
		_, _, pos = applySearchColoring(content, o.searchCtx.Query)

		if len(pos) > 0 {
			out[blockCtx.BlockNum] = true
		}
	}
	return out
}
