package output

import (
	"sort"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/jhump/protoreflect/dynamic"

	"github.com/streamingfast/substreams/manifest"
	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
	"github.com/streamingfast/substreams/tui2/common"
	"github.com/streamingfast/substreams/tui2/components/blockselect"
	"github.com/streamingfast/substreams/tui2/components/modsearch"
	"github.com/streamingfast/substreams/tui2/components/modselect"
	"github.com/streamingfast/substreams/tui2/components/search"
	"github.com/streamingfast/substreams/tui2/pages/request"
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

	blocksPerModule     map[string][]uint64
	payloads            map[request.BlockContext]*pbsubstreamsrpc.AnyModuleOutput
	bytesRepresentation dynamic.BytesRepresentation

	blockIDs map[uint64]string

	active            request.BlockContext // module + block
	outputViewYoffset map[request.BlockContext]int

	moduleSearchEnabled bool
	moduleSearchView    *modsearch.ModuleSearch
	//moduleSearchView
	outputModule string

	searchEnabled                   bool
	searchCtx                       *search.Search
	searchBlockNumsWithMatches      []uint64
	searchMatchingOutputViewOffsets []int
}

func New(c common.Common, manifestPath string, outputModule string) *Output {
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
		searchCtx:           search.New(c),
		bytesRepresentation: dynamic.BytesAsHex,
		moduleSearchView:    modsearch.New(c),
		outputModule:        outputModule,
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
	o.outputView.Height = h - 11
	o.moduleSearchView.SetSize(w, o.outputView.Height)
	outputViewTopBorder := 1
	o.outputView.Height = h - o.moduleSelector.Height - o.blockSelector.Height - outputViewTopBorder
	o.searchCtx.SetSize(w, h)
}

func (o *Output) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// WARN: this will not be so pretty for the reversible segment, as we're
	// flattening the block IDs into numbers...
	// Probably fine for now, as we're debugging the history.

	var cmds []tea.Cmd
	switch msg := msg.(type) {
	case search.SearchClearedMsg:
		o.searchEnabled = false
	case modsearch.DisableModuleSearch:
		o.moduleSearchEnabled = false
	case search.UpdateMatchingBlocks:
		o.searchBlockNumsWithMatches = o.orderMatchingBlocks(msg)
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
				if o.moduleSelector.AddModule(modName) {
					cmds = append(cmds, func() tea.Msg { return common.UpdateSeenModulesMsg(o.moduleSelector.Modules) })
				}
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
			o.setOutputViewContent()
		}

	case search.ApplySearchQueryMsg:
		o.setOutputViewContent()
		cmds = append(cmds, o.updateMatchingBlocks())
	case common.ModuleSelectedMsg:
		o.active.Module = string(msg)
		o.blockSelector.SetAvailableBlocks(o.blocksPerModule[o.active.Module])
		o.outputView.YOffset = o.outputViewYoffset[o.active]
		o.setOutputViewContent()
		cmds = append(cmds, o.updateMatchingBlocks())
	case blockselect.BlockChangedMsg:
		newBlock := uint64(msg)
		o.active.BlockNum = newBlock
		o.blockSelector.SetActiveBlock(newBlock)
		o.outputView.YOffset = o.outputViewYoffset[o.active]
		o.setOutputViewContent()
	case tea.KeyMsg:
		_, cmd := o.searchCtx.Update(msg)
		cmds = append(cmds, cmd)
		switch msg.String() {
		case "m":
			o.moduleSearchEnabled = true
			o.setOutputViewContent()
			return o, o.moduleSearchView.InitInput()
		case "/":
			o.searchEnabled = true
			cmds = append(cmds, o.searchCtx.InitInput())
		case "f":
			o.bytesRepresentation = (o.bytesRepresentation + 1) % 3
		case "N":
			for i := len(o.searchMatchingOutputViewOffsets) - 1; i >= 0; i-- {
				pos := o.searchMatchingOutputViewOffsets[i]
				if pos < o.outputView.YOffset {
					o.outputView.YOffset = pos
					break
				}
			}
		case "n":
			// msg was []int the list of matching positions.
			for _, pos := range o.searchMatchingOutputViewOffsets {
				if pos > o.outputView.YOffset {
					o.outputView.YOffset = pos
					break
				}
			}
		case "o":
			cmds = append(cmds, o.jumpToPreviousBlock())
		case "p":
			cmds = append(cmds, o.jumpToNextBlock())
		case "O":
			cmds = append(cmds, o.jumpToPreviousMatchingBlock())
		case "P":
			cmds = append(cmds, o.jumpToNextMatchingBlock())
		}
		o.outputViewYoffset[o.active] = o.outputView.YOffset
		o.setOutputViewContent()
	}

	_, cmd := o.moduleSearchView.Update(msg)
	cmds = append(cmds, cmd)

	_, cmd = o.moduleSelector.Update(msg)
	cmds = append(cmds, cmd)

	_, cmd = o.blockSelector.Update(msg)
	cmds = append(cmds, cmd)

	o.outputView, cmd = o.outputView.Update(msg)
	cmds = append(cmds, cmd)

	return o, tea.Batch(cmds...)
}

type displayContext struct {
	blockCtx          request.BlockContext
	searchViewEnabled bool
	searchQuery       string
	payload           *pbsubstreamsrpc.AnyModuleOutput
	searchJQMode      bool
}

func (o *Output) setOutputViewContent() {
	displayCtx := &displayContext{
		blockCtx:          o.active,
		searchViewEnabled: o.searchEnabled,
		searchQuery:       o.searchCtx.Current.Query,
		searchJQMode:      o.searchCtx.Current.JQMode,
		payload:           o.payloads[o.active],
	}
	if displayCtx != o.lastDisplayContext {
		vals := o.renderedOutput(displayCtx.payload, true)
		content := o.renderPayload(vals)
		if displayCtx.searchViewEnabled {
			var matchCount int
			var positions []int

			if displayCtx.searchJQMode {
				content, matchCount, positions = applyJQSearch(vals.plainJSON, o.searchCtx.Current.Query)
				content = highlightJSON(content)
			} else {
				content, matchCount, positions = applyKeywordSearch(content, o.searchCtx.Current.Query)
			}
			o.searchCtx.SetMatchCount(matchCount) //timesFound = lines
			o.searchMatchingOutputViewOffsets = positions
		}
		o.lastDisplayContext = displayCtx
		o.outputView.SetContent(content)
	}

}

func (o *Output) View() string {
	//curX, curY := o.getMatchingModuleIndexFromString(o.moduleSearchView.highlightedMod)

	var searchLine string
	if o.searchEnabled {
		searchLine = o.searchCtx.View()
	}
	o.setOutputViewContent()

	var middleBlock string
	if o.moduleSearchEnabled {
		middleBlock = o.moduleSearchView.View()
	} else {
		middleBlock = o.outputView.View()
	}

	out := lipgloss.JoinVertical(0,
		o.moduleSelector.View(),
		o.blockSelector.View(),
		"",
		middleBlock,
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

func (o *Output) updateMatchingBlocks() tea.Cmd {
	if !o.searchEnabled {
		return nil
	}
	matchingBlocks := o.searchAllBlocksForModule(o.active.Module)
	return func() tea.Msg {
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
		content := o.renderedOutput(payload, false)

		var count int
		if o.searchCtx.Current.JQMode {
			_, count, _ = applyJQSearch(content.plainJSON, o.searchCtx.Current.Query)
		} else {
			_, count, _ = applyKeywordSearch(content.plainLogs+content.plainJSON+content.plainOutput, o.searchCtx.Current.Query)
		}

		if count > 0 {
			out[blockCtx.BlockNum] = true
		}
	}
	return out
}

func (o *Output) orderMatchingBlocks(msg search.UpdateMatchingBlocks) []uint64 {
	l := make([]uint64, len(msg))
	count := 0
	for k := range msg {
		l[count] = k
		count++
	}
	sort.Slice(l, func(i, j int) bool { return l[i] < l[j] })
	return l
}

func (o *Output) jumpToPreviousBlock() tea.Cmd {
	withData := o.blocksPerModule[o.active.Module]
	activeBlockNum := o.active.BlockNum
	return func() tea.Msg {
		var prevIdx int
		for i, el := range withData {
			if el >= activeBlockNum {
				break
			}
			prevIdx = i
		}
		return blockselect.BlockChangedMsg(withData[prevIdx])
	}
}

func (o *Output) jumpToNextBlock() tea.Cmd {
	withData := o.blocksPerModule[o.active.Module]
	activeBlockNum := o.active.BlockNum
	return func() tea.Msg {
		var prevIdx = len(withData) - 1
		for i := prevIdx; i >= 0; i-- {
			el := withData[i]
			if el <= activeBlockNum {
				break
			}
			prevIdx = i
		}
		return blockselect.BlockChangedMsg(withData[prevIdx])
	}
}

func (o *Output) jumpToPreviousMatchingBlock() tea.Cmd {
	activeBlock := o.active.BlockNum
	blocks := o.searchBlockNumsWithMatches
	return func() tea.Msg {
		for i := len(blocks) - 1; i >= 0; i-- {
			block := blocks[i]
			if block < activeBlock {
				return blockselect.BlockChangedMsg(block)
			}
		}
		return nil
	}
}

func (o *Output) jumpToNextMatchingBlock() tea.Cmd {
	activeBlock := o.active.BlockNum
	blocks := o.searchBlockNumsWithMatches
	return func() tea.Msg {
		for _, block := range blocks {
			if block > activeBlock {
				return blockselect.BlockChangedMsg(block)
			}
		}
		return nil
	}
}

func (o *Output) getActiveModuleIndex() int {
	for i, mod := range o.moduleSelector.Modules {
		if mod == o.active.Module {
			return i
		}
	}
	return 0
}
