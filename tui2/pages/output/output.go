package output

import (
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/jhump/protoreflect/dynamic"
	"github.com/streamingfast/substreams/manifest"
	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
	"github.com/streamingfast/substreams/tui2/common"
	"github.com/streamingfast/substreams/tui2/components/blockselect"
	"github.com/streamingfast/substreams/tui2/components/modselect"
	"github.com/streamingfast/substreams/tui2/components/search"
	"github.com/streamingfast/substreams/tui2/pages/request"
	"sort"
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

	searchEnabled                   bool
	moduleSearchEnabled             bool
	moduleSearchView                *moduleSearchView
	outputModule                    string
	searchCtx                       *search.Search
	searchBlockNumsWithMatches      []uint64
	searchMatchingOutputViewOffsets []int
}

type moduleSearchView struct {
	moduleSearch     *search.ModuleSearch
	graphView        viewport.Model
	matchingModules  []string
	highlightedMod   string
	highlightedIndex int
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
		searchCtx:           search.NewSearch(c),
		bytesRepresentation: dynamic.BytesAsHex,
		moduleSearchView: &moduleSearchView{
			moduleSearch: search.NewModuleSearch(),
			graphView:    viewport.New(24, 79),
		},
		outputModule: outputModule,
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
	o.searchCtx.SetSize(w, h)
	
	o.moduleSearchView.graphView.Height = h - 12
	o.moduleSearchView.graphView.Width = o.outputView.Width
}

func (o *Output) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// WARN: this will not be so pretty for the reversible segment, as we're
	// flattening the block IDs into numbers...
	// Probably fine for now, as we're debugging the history.

	var cmds []tea.Cmd
	switch msg := msg.(type) {
	case search.UpdateModuleSearchQueryMsg:
		o.moduleSearchView.highlightedMod = ""
		o.moduleSearchView.moduleSearch.Query = string(msg)
		o.setViewportContent()
	case common.SelectedModuleChangeMsg:
		searchData := o.moduleSearchView
		switch msg {
		case "up":
			if !(searchData.highlightedIndex == 0) {
				searchData.highlightedMod = searchData.matchingModules[searchData.highlightedIndex-1]
				searchData.highlightedIndex--
			}
		case "down":
			if !(searchData.highlightedIndex == len(searchData.matchingModules)-1) {
				searchData.highlightedMod = searchData.matchingModules[searchData.highlightedIndex+1]
				searchData.highlightedIndex++
			}
		}
	case search.SearchClearedMsg:
		o.searchEnabled = false
	case search.ModuleSearchClearedMsg:
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
	case search.ApplyModuleSearchQueryMsg:
		o.moduleSearchEnabled = false
		o.active.Module = o.moduleSearchView.highlightedMod
		o.blockSelector.SetAvailableBlocks(o.blocksPerModule[o.active.Module])
		o.outputView.YOffset = o.outputViewYoffset[o.active]
		o.moduleSelector.Selected = o.getActiveModuleIndex()
		cmds = append(cmds, o.moduleSelector.DispatchModuleSelected)
		o.setViewportContent()
		cmds = append(cmds, o.updateMatchingBlocks())

	case modselect.ModuleSelectedMsg:
		o.active.Module = string(msg)
		o.blockSelector.SetAvailableBlocks(o.blocksPerModule[o.active.Module])
		o.outputView.YOffset = o.outputViewYoffset[o.active]
		o.setViewportContent()
		cmds = append(cmds, o.updateMatchingBlocks())
	case blockselect.BlockChangedMsg:
		newBlock := uint64(msg)
		o.active.BlockNum = newBlock
		o.blockSelector.SetActiveBlock(newBlock)
		o.outputView.YOffset = o.outputViewYoffset[o.active]
		o.setViewportContent()
	case tea.KeyMsg:
		_, cmd := o.searchCtx.Update(msg)
		cmds = append(cmds, cmd)
		switch msg.String() {
		case "m":
			if !o.searchEnabled {
				o.moduleSearchEnabled = true
				cmds = append(cmds, o.moduleSearchView.moduleSearch.InitInput())
				o.setViewportContent()
			}
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
		o.setViewportContent()
	}

	_, cmd := o.moduleSelector.Update(msg)
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

func containsPortions(s, query string) bool {
	queryIndex := 0
	for _, r := range s {
		if queryIndex < len(query) && r == rune(query[queryIndex]) {
			queryIndex++
		}
	}
	return queryIndex == len(query)
}

func (o *Output) setMatchingModules(query string) string {
	searchData := o.moduleSearchView
	matchingMods := make([]string, 0)
	maxHeight := o.outputView.Height - 3
	stringRows := make([]string, 0)

	for _, mod := range o.moduleSelector.Modules {
		if containsPortions(mod, query) {
			matchingMods = append(matchingMods, mod)
		}
	}

	if len(matchingMods) == 0 {
		return ""
	}

	if searchData.highlightedMod == "" {
		searchData.highlightedMod = matchingMods[0]
		searchData.highlightedIndex = 0
	}

	searchData.matchingModules = matchingMods
	matchingMods[searchData.highlightedIndex] = lipgloss.NewStyle().Foreground(lipgloss.Color("255")).Render(searchData.highlightedMod)

	tmp := make([][]string, 0)
	if len(matchingMods) > maxHeight {
		for i := 0; i < len(matchingMods); i += maxHeight {
			to := i + maxHeight
			if to > len(matchingMods) {
				to = len(matchingMods)
			}
			tmp = append(tmp, matchingMods[i:to])
		}
	} else {
		tmp = append(tmp, matchingMods)
	}

	for _, row := range tmp {
		stringRows = append(stringRows, lipgloss.JoinVertical(0, row...))
	}

	out := lipgloss.JoinHorizontal(0.5, stringRows...)
	return out
}

func (o *Output) setModuleSearchView(query string) string {

	o.moduleSearchView.graphView.SetContent(o.setMatchingModules(query))

	return lipgloss.JoinVertical(0,
		o.moduleSearchView.moduleSearch.View(),
		lipgloss.NewStyle().Border(lipgloss.NormalBorder(), true).Width(o.Width-2).Render(o.moduleSearchView.graphView.View()),
	)
}

func (o *Output) setViewportContent() {
	displayCtx := &displayContext{
		blockCtx:          o.active,
		searchViewEnabled: o.searchEnabled,
		searchQuery:       o.searchCtx.Current.Query,
		searchJQMode:      o.searchCtx.Current.JQMode,
		payload:           o.payloads[o.active],
	}
	if o.moduleSearchEnabled {
		o.outputView.SetContent(o.setModuleSearchView(o.moduleSearchView.moduleSearch.Query))
		return
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
	o.setViewportContent()
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
