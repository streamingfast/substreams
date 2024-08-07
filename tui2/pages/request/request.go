package request

import (
	"fmt"
	"log"
	"regexp"
	"strings"

	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
	"github.com/streamingfast/substreams/tui2/styles"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/tui2/common"
)

type BlockContext struct {
	Module   string
	BlockNum uint64
}

type Request struct {
	common.Common
	*Config

	form            *huh.Form
	inputEndpoint   *huh.Input
	inputModule     *huh.Select[string]
	inputStartBlock *huh.Input
	inputStopBlock  *huh.Input
	inputParams     *huh.Text
	inputConfirm    *huh.Confirm

	formStartBlock     string
	formStopBlock      string
	formModuleSelected string
	formEndpoint       string

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
	r.inputEndpoint = huh.NewInput().
		Key("endpoint").
		Value(&r.formEndpoint).
		Inline(false).
		Title("Endpoint (without https://):")
	r.inputModule = huh.NewSelect[string]().
		Key("module").
		Inline(true).
		Options(huh.NewOptions("loading...")...).
		Title("Top-level module to stream:")
	r.inputStartBlock = huh.NewInput().
		Key("start-block").
		Value(&r.formStartBlock).
		Inline(true).
		Validate(func(s string) error {
			if !regexp.MustCompile(`^\d+$`).MatchString(s) {
				return fmt.Errorf("specify only numbers")
			}
			return nil
		}).
		Description("Supports negative number, relative to head. Empty value defaults to module's start block.").
		Title("Start block:")
	r.inputStopBlock = huh.NewInput().
		Key("stop-block").
		Inline(true).
		Validate(func(s string) error {
			if s != "" {
				if !regexp.MustCompile(`^[\+\-]?\d+$`).MatchString(s) {
					return fmt.Errorf("specify only numbers, optionally prefixed by - or +")
				}
			}
			return nil
		}).
		Title("Stop block:").
		Description("Supports + prefix, relative to start block. Empty means never stop.")
	r.inputParams = huh.NewText().
		Key("params").
		//Inline(true).
		Validate(func(s string) error {
			if strings.TrimSpace(s) != "" {
				if !regexp.MustCompile(`^(\w+=[^,]+\n)*(\w+=[^,]+)$`).MatchString(s) {
					return fmt.Errorf("specify only key=value pairs separated by newlines")
				}
			}
			return nil
		}).
		Title("Per-module parameters:").
		Description("Specify using 'module_name=value', one per line.")
	r.inputConfirm = huh.NewConfirm().Affirmative("Run").Negative("").
		Key("confirm").
		Title("")
	r.form = huh.NewForm(huh.NewGroup(r.inputEndpoint, r.inputModule, r.inputStartBlock, r.inputStopBlock, r.inputParams, r.inputConfirm))
	return r.form.Init()
}

func (r *Request) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	formModel, cmd := r.form.Update(msg)
	if formModel != nil {
		r.form = formModel.(*huh.Form)
	}
	cmds = append(cmds, cmd)

	switch msg := msg.(type) {
	case NewRequestInstance:
		r.RequestSummary = msg.RequestSummary
		r.inputModule.Options(huh.NewOptions(r.Graph.MapModules()...)...)
		r.inputEndpoint.Value(&r.Config.Endpoint)
		r.inputModule.Value(&r.Config.OutputModule)
		r.inputStartBlock.Value(&r.Config.StartBlock)
		r.inputStopBlock.Value(&r.Config.StopBlock)
		r.inputParams.Value(&r.Config.Params)

	// case common.SetRequestValue:
	// 	log.Println("Received value", msg.Field, msg.Value)
	// 	switch msg.Field {
	// 	case "module":
	// 		r.Config.OutputModule = msg.Value
	// 	case "start-block":
	// 		r.Config.StartBlock = msg.Value
	// 	case "params":
	// 		r.Config.RawParams = strings.Split(msg.Value, "\n")
	// 	case "stop-block":
	// 		r.Config.StopBlock = msg.Value
	// 	case "endpoint":
	// 		r.Config.SubstreamsClientConfig.SetEndpoint(msg.Value)
	// 	}
	case common.ModuleSelectedMsg:
		log.Println("Setting selected module:", msg)
		r.Config.OutputModule = string(msg)

	case *pbsubstreamsrpc.SessionInit:
		r.traceId = msg.TraceId
		r.resolvedStartBlock = msg.ResolvedStartBlock
		r.linearHandoffBlock = msg.LinearHandoffBlock
	}
	return r, tea.Batch(cmds...)
}

func (r *Request) SetSize(w, h int) {
	r.Common.SetSize(w, h)
	summaryHeight := lipgloss.Height(r.renderRequestSummary())
	r.form.WithHeight(h - summaryHeight)
}

func (r *Request) View() string {
	return lipgloss.JoinVertical(lipgloss.Top,
		r.renderRequestSummary(),
		r.renderForm(),
	)
}
func (r *Request) renderForm() string {
	return r.form.View()
}

func (r *Request) renderRequestSummary() string {
	rows := [][]string{
		{"Package:", r.Config.ManifestPath},
		{"Endpoint:", r.Config.Endpoint},
		{"Network:", r.Config.OverrideNetwork},
		{"", ""},
		{"Module:", r.Config.OutputModule},
		{"Block range:", fmt.Sprintf("%s -> %s", r.Config.StartBlock, r.Config.StopBlock)},
		{"Module params:", r.Config.Params},
		{"", ""},
		{"Production mode:", fmt.Sprintf("%v", r.Config.ProdMode)},
	}
	if len(r.Config.DebugModulesInitialSnapshot) > 0 {
		rows = append(rows, []string{"Initial snapshots:", strings.Join(r.Config.DebugModulesInitialSnapshot, ", ")})
	}

	t := table.New().Border(lipgloss.Border{}).Width(r.Width - 2).StyleFunc(alternateCenteredTable).Rows(rows...)

	return lipgloss.NewStyle().Render(t.Render())
}

func alternateCenteredTable(row, col int) lipgloss.Style {
	color := styles.RequestOddRow
	if row%2 == 0 {
		color = styles.RequestEvenRow
	}
	if col == 0 {
		return color.Align(lipgloss.Right)
	}
	return color
}
