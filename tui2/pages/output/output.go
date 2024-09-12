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
	"github.com/streamingfast/substreams/tui2/components/blocksearch"
	"github.com/streamingfast/substreams/tui2/components/blockselect"
	"github.com/streamingfast/substreams/tui2/components/modsearch"
	"github.com/streamingfast/substreams/tui2/components/modselect"
	"github.com/streamingfast/substreams/tui2/components/search"
	"github.com/streamingfast/substreams/tui2/components/statusbar"
	"github.com/streamingfast/substreams/tui2/pages/request"
)

type Output struct {
	common.Common
	*request.Config

	msgDescs       map[string]*manifest.ModuleDescriptor
	messageFactory *dynamic.MessageFactory

	statusBar          *statusbar.StatusBar
	moduleSelector     *modselect.ModSelect
	blockSelector      *blockselect.BlockSelect
	outputView         viewport.Model
	lastDisplayContext *displayContext
	lastOutputContent  string

	lowBlock       *uint64
	highBlock      uint64
	firstBlockSeen bool

	blocksPerModule     map[string][]uint64
	payloads            map[request.BlockContext]*pbsubstreamsrpc.AnyModuleOutput
	bytesRepresentation dynamic.BytesRepresentation

	blockIDs map[uint64]string

	active            request.BlockContext // module + block
	outputViewYoffset map[request.BlockContext]int

	moduleSearchView *modsearch.ModuleSearch

	outputModule string
	logsEnabled  bool

	searchEnabled                   bool
	searchCtx                       *search.Search
	keywordToSearchFor              string
	searchBlockNumsWithMatches      []uint64
	searchMatchingOutputViewOffsets []int

	errReceived error

	blockSearchEnabled bool
	blockSearchCtx     *blocksearch.BlockSearch

	// moduleNavigatorMode bool
	// moduleNavigator     *modgraph.Navigator
}

func New(c common.Common, config *request.Config) (*Output, error) {
	// nav, err := modgraph.New(config.OutputModule, c, modgraph.WithModuleGraph(config.Graph))
	// if err != nil {
	// 	return nil, err
	// }

	output := &Output{
		Common:              c,
		Config:              config,
		blocksPerModule:     make(map[string][]uint64),
		payloads:            make(map[request.BlockContext]*pbsubstreamsrpc.AnyModuleOutput),
		blockIDs:            make(map[uint64]string),
		moduleSelector:      modselect.New(c, config.Graph),
		blockSelector:       blockselect.New(c),
		outputView:          viewport.New(24, 80),
		messageFactory:      dynamic.NewMessageFactoryWithDefaults(),
		outputViewYoffset:   map[request.BlockContext]int{},
		statusBar:           statusbar.New(c),
		searchCtx:           search.New(c),
		blockSearchCtx:      blocksearch.New(c),
		bytesRepresentation: dynamic.BytesAsBase64,
		moduleSearchView:    modsearch.New(c, "output"),
		outputModule:        config.OutputModule,
		logsEnabled:         true,
		//moduleNavigator:     nav,
		firstBlockSeen: true,
	}
	output.statusBar.SetShowLogs(output.logsEnabled)
	return output, nil
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
	o.statusBar.SetSize(w, h)
	o.moduleSearchView.SetSize(w, h)
	o.searchCtx.SetSize(w, h)

	//o.moduleNavigator.FrameHeight = h - 11
	outputViewTopBorder := 1
	o.outputView.Width = w
	o.outputView.Height = h - o.moduleSelector.Height - o.blockSelector.Height - outputViewTopBorder - o.statusBar.Height
}

func (o *Output) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// WARN: this will not be so pretty for the reversible segment, as we're
	// flattening the block IDs into numbers...
	// Probably fine for now, as we're debugging the history.

	var cmds []tea.Cmd
	switch msg := msg.(type) {
	case search.SearchClearedMsg:
		o.searchEnabled = false
		o.setOutputViewContent(true)
	case common.UpdateSeenModulesMsg:
		o.moduleSearchView.SetListItems(msg)
	case search.UpdateMatchingBlocks:
		o.searchBlockNumsWithMatches = o.orderMatchingBlocks(msg)
		o.blockSelector.Update(msg)
	case search.AddMatchingBlock:
		o.searchBlockNumsWithMatches = append(o.searchBlockNumsWithMatches, uint64(msg))
		o.blockSelector.Update(msg)
	case request.NewRequestInstance:
		o.errReceived = nil
		o.msgDescs = msg.MsgDescs

		// weird issue when the user rebuilds their substreams and runs a new request
		// the old logs would keep showing
		o.logsEnabled = false // reset logs

		// force a re-render of the output's view
		o.setOutputViewContent(true)
	case *pbsubstreamsrpc.BlockScopedData:
		blockNum := msg.Clock.Number

		if o.lowBlock == nil {
			o.lowBlock = &blockNum
		}
		if o.highBlock < blockNum {
			o.highBlock = blockNum
		}
		o.blockSelector.StretchBounds(*o.lowBlock, o.highBlock)

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

				if o.keywordToSearchFor != "" {
					if hasKeyword := o.searchIncomingBlockInModule(o.active.Module, blockNum); hasKeyword {
						cmds = append(cmds, func() tea.Msg {
							return search.AddMatchingBlock(blockNum)
						})
					}
				}
			}
			o.payloads[blockCtx] = output
			if o.firstBlockSeen {
				o.active = blockCtx
			}
			o.setOutputViewContent(false)
		}

	case search.ApplySearchQueryMsg:
		o.keywordToSearchFor = msg.Query
		o.setOutputViewContent(true)
		cmds = append(cmds, o.updateMatchingBlocks())
	case common.ModuleSelectedMsg:
		if msg.Target != "output" {
			break
		}
		o.active.Module = msg.ModuleName
		o.blockSelector.SetAvailableBlocks(o.blocksPerModule[o.active.Module])
		o.outputView.YOffset = o.outputViewYoffset[o.active]
		o.setOutputViewContent(true)
		cmds = append(cmds, o.updateMatchingBlocks())
		//_, _ = o.moduleNavigator.Update(msg)
		o.moduleSearchView.SetSelected(msg.ModuleName)

	case blockselect.BlockChangedMsg:
		if o.hasDataForBlock(uint64(msg)) {
			newBlock := uint64(msg)
			o.active.BlockNum = newBlock
			o.blockSelector.SetActiveBlock(newBlock)
			o.outputView.YOffset = o.outputViewYoffset[o.active]
			o.setOutputViewContent(true)
		} else {
			o.blockSearchEnabled = true
		}
	case tea.KeyMsg:
		_, cmd := o.searchCtx.Update(msg)
		cmds = append(cmds, cmd)
		switch msg.String() {
		case "M":
			//o.moduleNavigatorMode = !o.moduleNavigatorMode
			//o.setOutputViewContent(true)
		case "=":
			o.blockSearchEnabled = !o.blockSearchEnabled
			cmds = append(cmds, common.SetModalComponentCmd(o.blockSearchCtx))
		case "L":
			o.logsEnabled = !o.logsEnabled
			o.statusBar.SetShowLogs(o.logsEnabled)
			o.setOutputViewContent(true)
		case "m":
			o.setOutputViewContent(true)
			cmds = append(cmds, common.SetModalComponentCmd(o.moduleSearchView))
		case "/":
			o.searchEnabled = true
			cmds = append(cmds, common.SetModalComponentCmd(o.searchCtx))
		case "F":
			o.bytesRepresentation = (o.bytesRepresentation + 1) % 3
			o.statusBar.SetBytesRepresentation(o.bytesRepresentation)
			o.setOutputViewContent(true)
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
		case "g":
			cmds = append(cmds, o.jumpToFirstBlock())
		case "G":
			cmds = append(cmds, o.jumpToLastBlock())
		case "O":
			cmds = append(cmds, o.jumpToPreviousMatchingBlock())
		case "P":
			cmds = append(cmds, o.jumpToNextMatchingBlock())
		}
		o.outputViewYoffset[o.active] = o.outputView.YOffset
		o.setOutputViewContent(false)
	}

	_, _ = o.statusBar.Update(msg)

	var cmd tea.Cmd
	o.outputView, cmd = o.outputView.Update(msg)
	cmds = append(cmds, cmd)

	_, cmd = o.moduleSelector.Update(msg)
	cmds = append(cmds, cmd)

	return o, tea.Batch(cmds...)
}

type displayContext struct {
	blockCtx          request.BlockContext
	logsEnabled       bool
	searchViewEnabled bool
	searchQuery       string
	payload           *pbsubstreamsrpc.AnyModuleOutput
	searchJQMode      bool
	errReceived       error
}

func (o *Output) setOutputViewContent(forcedRender bool) {
	displayCtx := &displayContext{
		logsEnabled:       o.logsEnabled,
		blockCtx:          o.active,
		searchViewEnabled: o.searchEnabled,
		searchQuery:       o.searchCtx.Current.Query,
		searchJQMode:      o.searchCtx.Current.JQMode,
		payload:           o.payloads[o.active],
		errReceived:       o.errReceived,
	}

	if o.firstBlockSeen || forcedRender {
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

		o.lastOutputContent = content
		if content != "" {
			o.firstBlockSeen = false
		}
	} else {
		o.outputView.SetContent(o.lastOutputContent)
	}
}

func (o *Output) View() string {
	var searchLine string
	if o.searchEnabled {
		searchLine = o.searchCtx.View()
	}

	o.setOutputViewContent(false)

	middleSection := o.outputView.View()
	// TODO: reimplement the `navigator` module.

	out := lipgloss.JoinVertical(0,
		o.moduleSelector.View(),
		o.blockSelector.View(),
		"",
		middleSection,
		searchLine,
		o.statusBar.View(),
	)
	return out
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

func (o *Output) searchIncomingBlockInModule(moduleName string, block uint64) bool {
	var hasSearch bool

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
		hasSearch = true
	}

	return hasSearch
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
		if len(withData) <= prevIdx {
			return nil
		}
		return blockselect.BlockChangedMsg(withData[prevIdx])
	}
}

func (o *Output) jumpToFirstBlock() tea.Cmd {
	withData := o.blocksPerModule[o.active.Module]
	return func() tea.Msg {
		if len(withData) == 0 {
			return nil
		}
		return blockselect.BlockChangedMsg(withData[0])
	}
}

func (o *Output) jumpToLastBlock() tea.Cmd {
	withData := o.blocksPerModule[o.active.Module]
	return func() tea.Msg {
		if len(withData) == 0 {
			return nil
		}
		return blockselect.BlockChangedMsg(withData[len(withData)-1])
	}
}

func (o *Output) jumpToNextBlock() tea.Cmd {
	withData := o.blocksPerModule[o.active.Module]
	activeBlockNum := o.active.BlockNum
	return func() tea.Msg {
		var prevIdx = len(withData) - 1
		if prevIdx == -1 {
			return nil
		}
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

func (o *Output) hasDataForBlock(num uint64) bool {
	for _, b := range o.blockSelector.BlocksWithData {
		if b == num {
			return true
		}
	}
	return false
}
