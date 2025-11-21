package request

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/streamingfast/substreams/manifest"
	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
	"github.com/streamingfast/substreams/sink"
	"github.com/streamingfast/substreams/tui2/components/dataentry"
	"github.com/streamingfast/substreams/tui2/components/modsearch"
	"github.com/streamingfast/substreams/tui2/pages/build"
	"github.com/streamingfast/substreams/tui2/stream"
	"github.com/streamingfast/substreams/tui2/styles"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/tui2/common"
)

const LogoASCII = `
         ▄▄
     ▄▄██▀▀
▁▄▄██▀▀ ▁▄▄
▔▀▀█▆▄▄ ▔▀▀██▄▄
     ▀▀██▄▄▁ ▀▀██▄▄▁
         ▀▀▔ ▄▄▆█▀▀▔
         ▄▄██▀▀
         ▀▀
`

type Request struct {
	common.Common
	sinkerConfig *sink.SinkerConfig
	tuiConfig    *common.TUIConfig
	graph        *manifest.ModuleGraph

	isStreaming        bool
	RequestSummary     *Summary
	Modules            *pbsubstreams.Modules
	traceId            string
	resolvedStartBlock uint64
	linearHandoffBlock uint64
}

func New(c common.Common, sinkerConfig *sink.SinkerConfig, tuiConfig *common.TUIConfig) *Request {
	return &Request{
		Common:       c,
		sinkerConfig: sinkerConfig,
		tuiConfig:    tuiConfig,
	}
}

func (r *Request) Init() tea.Cmd {
	return nil
}

func (r *Request) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case NewRequestInstance:
		r.RequestSummary = msg.RequestSummary
		r.graph = msg.Graph

	case common.SetRequestValue:
		switch msg.Field {
		case "module":
			r.tuiConfig.OutputModule = msg.Value
		case "start-block":
			if value, err := strconv.ParseInt(msg.Value, 10, 64); err == nil {
				r.sinkerConfig.StartBlock = value
			}
		case "stop-block":
			r.tuiConfig.StopBlock = msg.Value
		case "limit-processed-blocks":
			if value, err := strconv.ParseUint(msg.Value, 10, 64); err == nil {
				r.sinkerConfig.LimitProcessedBlocks = value
			}
		case "endpoint":
			r.sinkerConfig.ClientConfig.SetEndpoint(msg.Value)
		case "params":
			// TODO: there's no interface to modify this for now, dataentry doesn't support it yet.
			r.tuiConfig.Params = msg.Value
		}

	case tea.KeyMsg:
		switch msg.String() {
		case "s":
			comp := dataentry.New(r.Common, "start-block", validateNumbersOnly)
			comp.Input.Prompt("Enter the start block number: ").
				Description("Block from which to start streaming. Numbers only. Negative means relative to chain head.\n\n")
			comp.SetValue(fmt.Sprintf("%d", r.tuiConfig.StartBlock))
			cmds = append(cmds, common.SetModalComponentCmd(comp))
		case "t":
			comp := dataentry.New(r.Common, "stop-block", validateNumberOrRelativeValue)
			comp.Input.Prompt("Enter the stop block number: ").
				Description("Enter numbers only, with an optional - or + prefix.\n\nYou can specify relative block numbers with - (to head) or + (to start block) prefixes.\n")
			comp.SetValue(r.tuiConfig.StopBlock)
			cmds = append(cmds, common.SetModalComponentCmd(comp))
		case "l":
			comp := dataentry.New(r.Common, "limit-processed-blocks", validateNumberOrRelativeValue)
			comp.Input.Prompt("Enter the number to limit processed blocks: ").
				Description("Enter numbers only, with an optional - or + prefix.\n")
			comp.SetValue(fmt.Sprintf("%d", r.sinkerConfig.LimitProcessedBlocks))
			cmds = append(cmds, common.SetModalComponentCmd(comp))
		case "m":
			comp := modsearch.New(r.Common, "request")
			comp.Title = "Select top-level map module (/ to filter)"
			if r.graph != nil {
				comp.SetListItems(r.graph.MapModules())
			}
			comp.SetSelected(r.tuiConfig.OutputModule)
			cmds = append(cmds, common.SetModalComponentCmd(comp))
		case "e":
			comp := dataentry.New(r.Common, "endpoint", nil)
			comp.Input.Prompt("Enter endpoint: ").
				Description("Without https://. Include port (:443). Find endpoints on https://thegraph.market\nExample: mainnet.eth.streamingfast.io:443\n")
			comp.SetValue(r.sinkerConfig.ClientConfig.Endpoint())
			cmds = append(cmds, common.SetModalComponentCmd(comp))
		case "a":
		case "p":
		case "enter":
			if r.tuiConfig.RequiresBuild {
				cmds = append(cmds, build.SetupNewBuildCmd())
			} else {
				if r.isStreaming {
					cmds = append(cmds, func() tea.Msg { return stream.InterruptStreamMsg })
				} else {
					r.isStreaming = true
					cmds = append(cmds, common.SetupNewInstanceCmd(true))
				}
			}
		}
	case common.ModuleSelectedMsg:
		if msg.Target == "request" {
			r.tuiConfig.OutputModule = msg.ModuleName
		}

	case *pbsubstreamsrpc.SessionInit:
		r.traceId = msg.TraceId
		r.resolvedStartBlock = msg.ResolvedStartBlock
		r.linearHandoffBlock = msg.LinearHandoffBlock
	case stream.StreamErrorMsg:
		r.isStreaming = false
	case stream.Msg:
		switch msg {
		case stream.EndOfStreamMsg:
			r.isStreaming = false
		}
	}
	return r, tea.Batch(cmds...)
}

func (r *Request) SetSize(w, h int) {
	r.Common.SetSize(w, h)
}

func (r *Request) View() string {
	return lipgloss.JoinVertical(lipgloss.Top,
		r.renderRequestSummary(),
	)
}

func (r *Request) calculateTUIStartBlock() {
	if r.graph != nil && r.tuiConfig.OutputModule != "" {
		if startBlockInt, err := r.graph.ModuleInitialBlock(r.tuiConfig.OutputModule); err == nil {
			r.tuiConfig.StartBlock = int64(startBlockInt)
		}
	}
}

func (r *Request) renderRequestSummary() string {
	if r.tuiConfig.StartBlock == 0 {
		r.calculateTUIStartBlock()
	}
	packageName := r.tuiConfig.ManifestPath
	packageMetaName := "unknown"
	packageMetaVersion := "unknown"
	if r.sinkerConfig.Pkg != nil && len(r.sinkerConfig.Pkg.PackageMeta) > 0 {
		packageMetaName = r.sinkerConfig.Pkg.PackageMeta[0].Name
		packageMetaVersion = r.sinkerConfig.Pkg.PackageMeta[0].Version
	}
	packageName = fmt.Sprintf("%s (%s-%s)", packageName, packageMetaName, packageMetaVersion)
	authToken := "No, run `substreams auth` to set it"
	if r.sinkerConfig.ClientConfig.AuthToken() != "" {
		authToken = "Yes"
	}
	rows := [][]string{
		{"Package:", packageName},
		{fmt.Sprintf("Endpoint %s:", styles.HelpKey.Render("<e>")), r.sinkerConfig.ClientConfig.Endpoint()},
		{"Auth Token loaded:", authToken},
		{"Network:", r.sinkerConfig.Network},
		{"Custom params:", r.tuiConfig.Params},
		{"Default params:", r.tuiConfig.DefaultParams},
		{"", ""},
		{fmt.Sprintf("Module %s:", styles.HelpKey.Render("<m>")), r.tuiConfig.OutputModule},
		{fmt.Sprintf("Start block %s:", styles.HelpKey.Render("<s>")), fmt.Sprintf("%d", r.tuiConfig.StartBlock)},
		{fmt.Sprintf("Stop block %s:", styles.HelpKey.Render("<t>")), r.tuiConfig.StopBlock},
		{fmt.Sprintf("Limit processed blocks %s:", styles.HelpKey.Render("<l>")), fmt.Sprintf("%d", r.sinkerConfig.LimitProcessedBlocks)},
	}
	if len(r.sinkerConfig.DevOutputSnapshots) > 0 {
		rows = append(rows,
			[]string{"Initial snapshots:", strings.Join(r.sinkerConfig.DevOutputSnapshots, ", ")},
		)
	}
	if r.sinkerConfig.Mode == sink.SubstreamsModeProduction {
		rows = append(rows,
			[]string{"Production mode:", "true"},
		)
	} else {
		printedModules := "[all non-imported modules]"
		if r.sinkerConfig.DevOutputModules != nil {
			printedModules = strings.Join(r.sinkerConfig.DevOutputModules, ",")
		}
		rows = append(rows,
			[]string{"Printed modules:", printedModules},
		)

	}
	rows = append(rows,
		[]string{"", ""},
	)

	if r.tuiConfig.RequiresBuild {
		rows = append(rows, []string{"You must first build your substreams package:", styles.BuildButton.Render("BUILD <enter>")})
	} else {
		if r.isStreaming {
			rows = append(rows, []string{"", styles.StreamButtonStop.Render("STOP <enter>")})
		} else {
			rows = append(rows, []string{"", styles.StreamButtonStart.Render("STREAM <enter>")})
		}
	}

	rows = append(rows,
		[]string{"", ""},
		[]string{"", styles.LogoASCII.Render(LogoASCII)},
	)

	t := table.New().Border(lipgloss.Border{}).Width(r.Width - 2).StyleFunc(alternateCenteredTable).Rows(rows...)

	return lipgloss.NewStyle().Height(r.Height).MaxHeight(r.Height).Render(t.Render())
}

func alternateCenteredTable(row, col int) lipgloss.Style {
	color := styles.RequestOddRow
	// if row%2 == 0 {
	// 	color = styles.RequestEvenRow
	// }
	if col == 0 {
		return color.Align(lipgloss.Right)
	}
	return color
}
