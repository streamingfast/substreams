package output

import (
	"fmt"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	"github.com/jhump/protoreflect/dynamic"

	"github.com/streamingfast/substreams/manifest"
	"github.com/streamingfast/substreams/tui2/components/blockselect"

	"github.com/streamingfast/substreams/tui2/components/modselect"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/streamingfast/substreams/tui2/common"
)

var badSearchChars = map[tea.KeyType]bool{
	tea.KeyTab:  true,
	tea.KeyDown: true,
	tea.KeyUp:   true,
}

type searchCtx struct {
	searchKeyword string
	timesFound    int

	resultViewEnabled bool
	searchVisible     bool

	searchFocused bool
}

type viewCtxStructure struct {
	payload string
}
type viewContextMap map[string]map[uint64]*viewCtxStructure

type Output struct {
	common.Common

	msgDescs       map[string]*manifest.ModuleDescriptor
	modules        map[string]*pbsubstreams.Module
	messageFactory *dynamic.MessageFactory

	moduleSelector    *modselect.ModSelect
	blockSelector     *blockselect.BlockSelect
	outputView        viewport.Model
	lastOutputContent *pbsubstreams.ModuleOutput

	lowBlock  uint64
	highBlock uint64

	blocksPerModule map[string][]uint64
	payloads        map[string]map[uint64]*pbsubstreams.ModuleOutput
	blockIDs        map[uint64]string

	activeModule  string
	activeBlock   uint64
	searchInput   textinput.Model
	searchCtx     searchCtx
	outputViewCtx viewContextMap
}

func New(c common.Common, msgDescs map[string]*manifest.ModuleDescriptor, modules *pbsubstreams.Modules) *Output {
	mods := map[string]*pbsubstreams.Module{}
	for _, mod := range modules.Modules {
		mods[mod.Name] = mod
	}

	output := &Output{
		Common:          c,
		msgDescs:        msgDescs,
		modules:         mods,
		blocksPerModule: make(map[string][]uint64),
		payloads:        make(map[string]map[uint64]*pbsubstreams.ModuleOutput),
		blockIDs:        make(map[uint64]string),
		moduleSelector:  modselect.New(c),
		blockSelector:   blockselect.New(c),
		outputView:      viewport.New(24, 80),
		messageFactory:  dynamic.NewMessageFactoryWithDefaults(),
	}
	output.searchInput = textinput.New()
	output.searchInput.Placeholder = "Search"
	output.searchInput.Focus()
	output.searchInput.Prompt = ""
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
		//o.setViewContext()
		o.activeModule = string(msg)
		o.blockSelector.SetAvailableBlocks(o.blocksPerModule[o.activeModule])
		o.setViewportContent()

	case blockselect.BlockSelectedMsg:
		//o.setViewContext()
		o.activeBlock = uint64(msg)
		o.setViewportContent()
	case tea.KeyMsg:
		if msg.String() == "/" {

			// toggle search visibility
			o.searchCtx.searchVisible = !o.searchCtx.searchVisible

			if searchCtx.resultViewEnabled {
				searchCtx.resultViewEnabled = false
			}

			// Set to focus or blur
			if o.searchCtx.searchVisible {
				o.searchCtx.searchFocused = true
				o.searchInput.Focus()
			} else {
				searchCtx.searchFocused = false
				o.searchInput.Blur()
			}
		} else if searchCtx.searchVisible {
			if o.searchInput.Focused() {
				_, cmd := o.searchInput.Update(msg)
				if msg.Type == tea.KeyEnter {

					// Update search context
					keyword := o.searchInput.Value()
					searchCtx.searchKeyword = keyword
					searchCtx.resultViewEnabled = true
					searchCtx.searchVisible = false

					//set highlighted payload with count on the entire block
					if o.lastOutputContent != nil {
						payloadIn := o.renderPayload(o.lastOutputContent)
						fullBlockIn := o.renderPayload(o.payloads[o.activeModule][o.activeBlock])
						payloadOut, count := applySearchColoring(fullBlockIn, payloadIn, searchCtx.searchKeyword)
						searchCtx.timesFound = count
						o.outputView.SetContent(payloadOut)

					}

				} else if msg.Type == tea.KeyLeft {
					o.searchInput.SetCursor(o.searchInput.Position() - 1)
				} else if msg.Type == tea.KeyRight {
					o.searchInput.SetCursor(o.searchInput.Position() + 1)
				} else if msg.Type == tea.KeyBackspace {
					o.searchInput.SetCursor(o.searchInput.Position() - 1)
					o.searchInput.SetValue(o.searchInput.Value()[:o.searchInput.Position()])
				} else if badSearchChars[msg.Type] != true {
					o.searchInput.SetValue(fmt.Sprintf("%s%s", o.searchInput.Value(), msg))
					o.searchInput.SetCursor(o.searchInput.Position() + 2)
				}

				cmds = append(cmds, cmd)
			}
		} else {
			_, cmd := o.moduleSelector.Update(msg)
			cmds = append(cmds, cmd)
			_, cmd = o.blockSelector.Update(msg)
			cmds = append(cmds, cmd)
			o.outputView, cmd = o.outputView.Update(msg)
			cmds = append(cmds, cmd)
		}
	}
	return o, tea.Batch(cmds...)
}

func (o *Output) setViewportContent() {
	if mod, found := o.payloads[o.activeModule]; found {
		if payload, found := mod[o.activeBlock]; found {
			// Do the decoding once per view, and cache the decoded value if it hasn't changed
			if payload != o.lastOutputContent {

				//if o.outputViewCtx[o.activeModule][o.activeBlock] != nil {
				//	ctx := o.outputViewCtx[o.activeModule][o.activeBlock]
				//	o.outputView.SetContent(ctx.payload)
				//}

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

func (o *Output) setViewContext() string {
	viewCtx := &o.outputViewCtx

	payloadIn := o.renderPayload(o.lastOutputContent)
	fullBlockIn := o.renderPayload(o.payloads[o.activeModule][o.activeBlock])
	payloadOut, _ := applySearchColoring(fullBlockIn, payloadIn, "")

	// Update view context
	if len(*viewCtx) == 0 {
		*viewCtx = map[string]map[uint64]*viewCtxStructure{
			o.activeModule: {
				o.activeBlock: &viewCtxStructure{
					payload: payloadOut,
				},
			},
		}
	} else {
		o.outputViewCtx[o.activeModule][o.activeBlock] = &viewCtxStructure{
			payload: payloadOut,
		}
	}

	return payloadOut
}
func (o *Output) displaySearchOutput() string {
	out := ""
	ctx := o.searchCtx

	if ctx.resultViewEnabled {
		// display relevant search results
		_, timesFound := applySearchColoring(o.renderPayload(o.payloads[o.activeModule][o.activeBlock]), o.renderPayload(o.lastOutputContent), ctx.searchKeyword)
		return fmt.Sprintf("%s - (%v instances found)", ctx.searchKeyword, timesFound)
	}
	if ctx.searchVisible {
		return o.searchInput.View()
	}
	return out
}

func (o *Output) displayOutputView() string {
	out := o.outputView.View()
	viewCtxMap := o.outputViewCtx
	viewCtx := viewCtxMap[o.activeModule][o.activeBlock]

	// If block has a search context
	if viewCtx != nil {
		o.outputView.SetContent(viewCtx.payload)
		out = o.outputView.View()
	} else {

		// if results is active
		if o.searchCtx.resultViewEnabled {

			// gather block info and apply highlighting
			payloadIn := o.renderPayload(o.lastOutputContent)
			fullBlockIn := o.renderPayload(o.payloads[o.activeModule][o.activeBlock])
			payloadHighlighted, _ := applySearchColoring(fullBlockIn, payloadIn, o.searchCtx.searchKeyword)

			o.outputView.SetContent(payloadHighlighted)
			out = o.outputView.View()
		}
	}

	return out
}

func (o *Output) View() string {

	out := lipgloss.JoinVertical(0,
		o.moduleSelector.View(),
		o.blockSelector.View(),
		"",
		o.displayOutputView(),
		o.displaySearchOutput(),
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
	}
}

func (o *Output) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		o.ShortHelp(),
	}
}
