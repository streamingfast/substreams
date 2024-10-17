package request

import (
	"fmt"
	"strings"

	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
	"github.com/streamingfast/substreams/tui2/components/dataentry"
	"github.com/streamingfast/substreams/tui2/components/modsearch"
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

type BlockContext struct {
	Module   string
	BlockNum uint64
}

type Request struct {
	common.Common
	*Config

	isStreaming        bool
	RequestSummary     *Summary
	Modules            *pbsubstreams.Modules
	traceId            string
	resolvedStartBlock uint64
	linearHandoffBlock uint64
}

func New(c common.Common, conf *Config) *Request {
	return &Request{
		Common: c,
		Config: conf,
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

	case common.SetRequestValue:
		switch msg.Field {
		case "module":
			r.Config.OutputModule = msg.Value
		case "start-block":
			r.Config.StartBlock = msg.Value
			if strings.HasPrefix(msg.Value, "-") {
				r.Config.StopBlock = ""
			}
		case "stop-block":
			r.Config.StopBlock = msg.Value
		case "endpoint":
			r.Config.Endpoint = msg.Value
		case "params":
			// TODO: there's no interface to modify this for now, dataentry doesn't support it yet.
			r.Config.Params = msg.Value
		}

	case tea.KeyMsg:
		switch msg.String() {
		case "s":
			comp := dataentry.New(r.Common, "start-block", validateNumbersOnly)
			comp.Input.Prompt("Enter the start block number: ").
				Description("Block from which to start streaming. Numbers only. Negative means relative to chain head.\n\n")
			comp.SetValue(r.Config.StartBlock)
			cmds = append(cmds, common.SetModalComponentCmd(comp))
		case "t":
			comp := dataentry.New(r.Common, "stop-block", validateNumberOrRelativeValue)
			comp.Input.Prompt("Enter the stop block number: ").
				Description("Enter numbers only, with an optional - or + prefix.\n\nYou can specify relative block numbers with - (to head) or + (to start block) prefixes.\n")
			comp.SetValue(r.Config.StopBlock)
			cmds = append(cmds, common.SetModalComponentCmd(comp))
		case "m":
			comp := modsearch.New(r.Common, "request")
			comp.Title = "Select top-level map module (/ to filter)"
			comp.SetListItems(r.Config.Graph.MapModules())
			comp.SetSelected(r.Config.OutputModule)
			cmds = append(cmds, common.SetModalComponentCmd(comp))
		case "e":
			comp := dataentry.New(r.Common, "endpoint", nil)
			comp.Input.Prompt("Enter endpoint: ").
				Description("Without https://. Include port (:443). Find endpoints on https://thegraph.market\nExample: mainnet.eth.streamingfast.io:443\n")
			comp.SetValue(r.Config.Endpoint)
			cmds = append(cmds, common.SetModalComponentCmd(comp))
		case "a":
		case "p":
		case "enter":
			if r.isStreaming {
				cmds = append(cmds, func() tea.Msg { return stream.InterruptStreamMsg })
			} else {
				r.isStreaming = true
				cmds = append(cmds, SetupNewInstanceCmd(true))
			}
		}
	case common.ModuleSelectedMsg:
		if msg.Target == "request" {
			r.Config.OutputModule = msg.ModuleName
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

func (r *Request) renderRequestSummary() string {
	startBlock := r.Config.StartBlock
	if startBlock == "" && r.Config.Graph != nil {
		startBlockInt, _ := r.Config.Graph.ModuleInitialBlock(r.Config.OutputModule)
		startBlock = fmt.Sprintf("%d (module's initial block)", startBlockInt)
	}
	packageName := r.Config.ManifestPath
	packageMetaName := "unknown"
	packageMetaVersion := "unknown"
	if r.Config.Pkg != nil && len(r.Config.Pkg.PackageMeta) > 0 {
		packageMetaName = r.Config.Pkg.PackageMeta[0].Name
		packageMetaVersion = r.Config.Pkg.PackageMeta[0].Version
	}
	packageName = fmt.Sprintf("%s (%s-%s)", packageName, packageMetaName, packageMetaVersion)
	authToken := "No, run `substreams auth` to set it"
	if r.Config.SubstreamsClientConfig.AuthToken() != "" {
		authToken = "Yes"
	}
	rows := [][]string{
		{"Package:", packageName},
		{fmt.Sprintf("Endpoint %s:", styles.HelpKey.Render("<e>")), r.Config.Endpoint},
		{"Auth Token loaded:", authToken},
		{"Network:", r.Config.OverrideNetwork},
		{"Custom params:", r.Config.Params},
		{"Default params:", r.Config.DefaultParams},
		{"", ""},
		{fmt.Sprintf("Module %s:", styles.HelpKey.Render("<m>")), r.Config.OutputModule},
		{fmt.Sprintf("Start block %s:", styles.HelpKey.Render("<s>")), startBlock},
		{fmt.Sprintf("Stop block %s:", styles.HelpKey.Render("<t>")), r.Config.StopBlock},
	}
	if len(r.Config.DebugModulesInitialSnapshot) > 0 {
		rows = append(rows,
			[]string{"Initial snapshots:", strings.Join(r.Config.DebugModulesInitialSnapshot, ", ")},
		)
	}
	if r.Config.ProdMode {
		rows = append(rows,
			[]string{"Production mode:", fmt.Sprintf("%v", r.Config.ProdMode)},
		)
	}
	rows = append(rows,
		[]string{"", ""},
	)

	if r.isStreaming {
		rows = append(rows, []string{"", styles.StreamButtonStop.Render("STOP <enter>")})
	} else {
		rows = append(rows, []string{"", styles.StreamButtonStart.Render("STREAM <enter>")})
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
