package output

import (
	"fmt"
	"log"

	"github.com/charmbracelet/bubbles/key"

	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/jhump/protoreflect/dynamic"

	"github.com/streamingfast/substreams/manifest"
	"github.com/streamingfast/substreams/tui2/common"
	"github.com/streamingfast/substreams/tui2/components/blockselect"
	"github.com/streamingfast/substreams/tui2/components/modselect"
)

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

	blocksPerModule map[string][]uint64
	payloads        map[blockContext]*pbsubstreamsrpc.AnyModuleOutput

	blockIDs map[uint64]string

	active            blockContext // module + block
	outputViewYoffset map[blockContext]int
	searchInput       textinput.Model
	searchCtx         searchCtx
}

type searchCtx struct {
	enabled bool

	searchKeyword string

	timesFound     int
	matchPositions []int
}

type blockContext struct {
	module   string
	blockNum uint64
}

func New(c common.Common, msgDescs map[string]*manifest.ModuleDescriptor, modules *pbsubstreams.Modules) *Output {
	mods := map[string]*pbsubstreams.Module{}
	for _, mod := range modules.Modules {
		mods[mod.Name] = mod
	}

	output := &Output{
		Common:            c,
		msgDescs:          msgDescs,
		blocksPerModule:   make(map[string][]uint64),
		payloads:          make(map[blockContext]*pbsubstreamsrpc.AnyModuleOutput),
		blockIDs:          make(map[uint64]string),
		moduleSelector:    modselect.New(c),
		blockSelector:     blockselect.New(c),
		outputView:        viewport.New(24, 80),
		messageFactory:    dynamic.NewMessageFactoryWithDefaults(),
		outputViewYoffset: map[blockContext]int{},
	}
	output.searchInput = textinput.New()
	output.searchInput.Placeholder = ""
	output.searchInput.Prompt = "/"
	output.searchInput.CharLimit = 256
	output.searchInput.Width = 80
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
	searchCtx := &o.searchCtx

	var cmds []tea.Cmd
	switch msg := msg.(type) {
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
			blockCtx := blockContext{
				module:   modName,
				blockNum: blockNum,
			}

			if _, found := o.payloads[blockCtx]; !found {
				o.moduleSelector.AddModule(modName)
				if o.active.module == "" {
					o.active.module = modName
					o.active.blockNum = blockNum
				}
				o.blocksPerModule[modName] = append(o.blocksPerModule[modName], blockNum)
				if modName == o.active.module {
					o.blockSelector.SetAvailableBlocks(o.blocksPerModule[modName])
				}
			}
			o.payloads[blockCtx] = output
			o.setViewportContent()
		}

	case modselect.ModuleSelectedMsg:
		//o.setViewContext()
		o.active.module = string(msg)
		o.blockSelector.SetAvailableBlocks(o.blocksPerModule[o.active.module])
		o.outputView.YOffset = o.outputViewYoffset[o.active]
		o.setViewportContent()

	case blockselect.BlockSelectedMsg:
		//o.setViewContext()
		o.active.blockNum = uint64(msg)
		o.outputView.YOffset = o.outputViewYoffset[o.active]
		o.setViewportContent()
	case tea.KeyMsg:
		if searchCtx.enabled && o.searchInput.Focused() {
			switch msg.String() {
			case "enter":
				keyword := o.searchInput.Value()
				searchCtx.searchKeyword = keyword
				o.searchInput.Blur()

				//if keyword == "" {
				//	// alternative: match the ESC key when
				//	searchCtx.searchVisible = false
				//}
			case "backspace":
				if o.searchInput.Value() == "" {
					searchCtx.enabled = false
				}
			}

			var cmd tea.Cmd
			o.searchInput, cmd = o.searchInput.Update(msg)
			cmds = append(cmds, cmd)

		} else {
			switch msg.String() {
			case "/":
				o.searchCtx.enabled = true
				o.searchInput.Focus()
				o.searchInput.SetValue("")
			case "n":
				log.Println("n pressed")
				// update the offset based on the `positions`
				for _, pos := range o.searchCtx.matchPositions {
					if pos > o.outputView.YOffset {
						o.outputView.YOffset = pos
						break
					}
				}
			case "N":
				log.Println("N pressed")
				for i := len(o.searchCtx.matchPositions) - 1; i >= 0; i-- {
					pos := o.searchCtx.matchPositions[i]
					if pos < o.outputView.YOffset {
						o.outputView.YOffset = pos
						break
					}
				}
			}

			_, cmd := o.moduleSelector.Update(msg)
			cmds = append(cmds, cmd)

			_, cmd = o.blockSelector.Update(msg)
			cmds = append(cmds, cmd)

			o.outputView, cmd = o.outputView.Update(msg)
			cmds = append(cmds, cmd)
			o.outputViewYoffset[o.active] = o.outputView.YOffset
		}
		o.setViewportContent()
	}
	return o, tea.Batch(cmds...)
}

type displayContext struct {
	blockCtx          blockContext
	searchViewEnabled bool
	searchKeyword     string
	payload           *pbsubstreamsrpc.AnyModuleOutput
}

func (o *Output) setViewportContent() {
	dpContext := &displayContext{
		blockCtx:          o.active,
		searchViewEnabled: o.searchCtx.enabled,
		searchKeyword:     o.searchCtx.searchKeyword,
		payload:           o.payloads[o.active],
	}

	if dpContext != o.lastDisplayContext {
		content := o.renderPayload(dpContext.payload)
		if dpContext.searchViewEnabled {
			var lines int
			var positions []int
			content, lines, positions = applySearchColoring(content, o.searchCtx.searchKeyword)
			o.searchCtx.timesFound = lines
			o.searchCtx.matchPositions = positions
		}
		o.lastDisplayContext = dpContext
		o.outputView.SetContent(content)
	}

}

func (o *Output) View() string {
	out := lipgloss.JoinVertical(0,
		o.moduleSelector.View(),
		o.blockSelector.View(),
		"",
		o.outputView.View(),
		o.displaySearchOutput(),
	)
	return out
}

func (o *Output) displaySearchOutput() string {
	ctx := o.searchCtx
	if ctx.enabled {
		if !o.searchInput.Focused() {
			return fmt.Sprintf("/%s - (%v instances found)", ctx.searchKeyword, ctx.timesFound)
		} else {
			return o.searchInput.View()
		}
	}
	return ""
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
	}
}

func (o *Output) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		o.ShortHelp(),
	}
}
