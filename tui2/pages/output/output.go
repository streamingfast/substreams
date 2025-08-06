package output

import (
	"fmt"
	"slices"
	"sort"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/jhump/protoreflect/dynamic"

	"github.com/streamingfast/substreams/manifest"
	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
	"github.com/streamingfast/substreams/protodecode"
	"github.com/streamingfast/substreams/sink"
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
	sinkerConfig *sink.SinkerConfig
	tuiConfig    *common.TUIConfig
	graph        *manifest.ModuleGraph

	decoder        *protodecode.Decoder
	msgDescs       map[string]*manifest.ModuleDescriptor
	messageFactory *dynamic.MessageFactory

	statusBar          *statusbar.StatusBar
	moduleSelector     *modselect.ModSelect
	blockSelector      *blockselect.BlockSelect
	outputView         viewport.Model
	lastDisplayContext *displayContext
	lastOutputContent  string

	lowBlock  *uint64
	highBlock uint64

	blocksPerModule     map[string][]uint64
	payloads            map[common.BlockContext]*pbsubstreamsrpc.AnyModuleOutput
	bytesRepresentation dynamic.BytesRepresentation

	blockIDs map[uint64]string

	active            common.BlockContext // module + block
	outputViewYoffset map[common.BlockContext]int

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

func New(c common.Common, sinkerConfig *sink.SinkerConfig, tuiConfig *common.TUIConfig) (*Output, error) {
	bytesRepresentation := common.ToDynamicBytesRepresentation(sink.InferBytesRepresentation(sinkerConfig.Pkg.Network, sinkerConfig.ClientConfig.Endpoint()))
	output := &Output{
		Common:              c,
		sinkerConfig:        sinkerConfig,
		tuiConfig:           tuiConfig,
		blocksPerModule:     make(map[string][]uint64),
		payloads:            make(map[common.BlockContext]*pbsubstreamsrpc.AnyModuleOutput),
		blockIDs:            make(map[uint64]string),
		blockSelector:       blockselect.New(c),
		outputView:          viewport.New(24, 80),
		messageFactory:      dynamic.NewMessageFactoryWithDefaults(),
		outputViewYoffset:   map[common.BlockContext]int{},
		statusBar:           newStatusBarWithBytesRepresentation(c, bytesRepresentation),
		searchCtx:           search.New(c),
		blockSearchCtx:      blocksearch.New(c),
		bytesRepresentation: bytesRepresentation,
		moduleSearchView:    modsearch.New(c, "output"),
		outputModule:        tuiConfig.OutputModule,
		logsEnabled:         true,
		//moduleNavigator:     nav,
	}
	output.statusBar.SetShowLogs(output.logsEnabled)
	return output, nil
}

func (o *Output) Init() tea.Cmd {
	//o.outputView.HighPerformanceRendering = true
	var cmds []tea.Cmd
	if o.moduleSelector != nil {
		cmds = append(cmds, o.moduleSelector.Init())
	}
	cmds = append(cmds, o.blockSelector.Init())
	return tea.Batch(cmds...)
}

func (o *Output) SetSize(w, h int) {
	o.Common.SetSize(w, h)
	if o.moduleSelector != nil {
		o.moduleSelector.SetSize(w, 2)
	}
	o.blockSelector.SetSize(w, 5)
	o.statusBar.SetSize(w, h)
	o.moduleSearchView.SetSize(w, h)
	o.searchCtx.SetSize(w, h)

	//o.moduleNavigator.FrameHeight = h - 11
	outputViewTopBorder := 1
	o.outputView.Width = w
	moduleSelectorHeight := 0
	if o.moduleSelector != nil {
		moduleSelectorHeight = o.moduleSelector.Height
	}
	o.outputView.Height = h - moduleSelectorHeight - o.blockSelector.Height - outputViewTopBorder - o.statusBar.Height
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
		o.graph = msg.Graph

		// Initialize module selector now that we have the graph
		if o.graph != nil {
			o.moduleSelector = modselect.New(o.Common, o.graph)
			o.moduleSelector.SetSize(o.Width, 2)
		}

		// Initialize decoder with the manifest descriptors
		decoder, err := protodecode.NewDecoderFromManifest(o.sinkerConfig.Pkg, msg.MsgDescs)
		if err != nil {
			o.errReceived = fmt.Errorf("failed to create decoder: %w", err)
		} else {
			o.decoder = decoder
		}

		o.blocksPerModule = make(map[string][]uint64)
		o.payloads = make(map[common.BlockContext]*pbsubstreamsrpc.AnyModuleOutput)
		o.blockIDs = make(map[uint64]string)
		o.blockSelector.Update(blockselect.NewRequestInstanceMsg{})

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

		// this will run on first received data (whatever module)
		// we always add the "output module" as soon as we get data
		if o.moduleSelector != nil && o.moduleSelector.AddModule(o.outputModule) {
			cmds = append(cmds, func() tea.Msg { return common.UpdateSeenModulesMsg(o.moduleSelector.Modules) })
			o.active.Module = o.outputModule
			o.active.BlockNum = blockNum
		}

		o.blockIDs[msg.Clock.Number] = msg.Clock.Id
		for _, output := range msg.AllModuleOutputs() {
			if output.IsEmpty() {
				continue
			}

			modName := output.Name()
			blockCtx := common.BlockContext{
				Module:   modName,
				BlockNum: blockNum,
			}

			forceRedraw := false
			if _, found := o.payloads[blockCtx]; !found {
				if o.moduleSelector != nil && modName != "" && o.moduleSelector.AddModule(modName) {
					cmds = append(cmds, func() tea.Msg { return common.UpdateSeenModulesMsg(o.moduleSelector.Modules) })
				}
				if o.active.Module == "" {
					o.active.Module = modName
					o.active.BlockNum = blockNum
				}
				if o.active.Module == modName && len(o.blocksPerModule[modName]) == 0 {
					forceRedraw = true
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
			o.setOutputViewContent(forceRedraw)
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
			o.bytesRepresentation = (o.bytesRepresentation + 1) % 4
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

	if o.moduleSelector != nil {
		_, cmd = o.moduleSelector.Update(msg)
		cmds = append(cmds, cmd)
	}

	return o, tea.Batch(cmds...)
}

type displayContext struct {
	blockCtx          common.BlockContext
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

	if forcedRender {
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

	var moduleSelectorView string
	if o.moduleSelector != nil {
		moduleSelectorView = o.moduleSelector.View()
	}
	out := lipgloss.JoinVertical(0,
		moduleSelectorView,
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
		blockCtx := common.BlockContext{
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

	blockCtx := common.BlockContext{
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
	return slices.Contains(o.blockSelector.BlocksWithData, num)
}

func newStatusBarWithBytesRepresentation(c common.Common, bytesRepresentation dynamic.BytesRepresentation) *statusbar.StatusBar {
	statusBar := statusbar.New(c)
	statusBar.SetBytesRepresentation(bytesRepresentation)
	return statusBar
}
