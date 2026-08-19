package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/fatih/color"
	"github.com/mattn/go-isatty"
	"github.com/muesli/termenv"
	"github.com/olekukonko/tablewriter"
	"github.com/olekukonko/tablewriter/renderer"
	"github.com/olekukonko/tablewriter/tw"
	"github.com/spf13/cobra"
	"github.com/streamingfast/cli"
	"github.com/streamingfast/cli/sflags"
	"github.com/streamingfast/logging"
	"github.com/streamingfast/substreams/sink"
	"go.uber.org/zap"
)

var (
	// Styling for output - colors only enabled if terminal is TTY
	isTTY         = isatty.IsTerminal(os.Stdout.Fd())
	titleStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	headerStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("14"))
	labelStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	valueStyle    = lipgloss.NewStyle().Bold(true)
	successStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	errorStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	dimStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	noteStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("238")).Italic(true)
	separatorChar = "─"

	sectionSeparator = strings.Repeat(separatorChar, 105)
)

func init() {
	// Disable colors if not a TTY
	if !isTTY {
		lipgloss.SetColorProfile(termenv.Ascii)
	}

	sink.AddFlagsToSet(estimateCmd.Flags(),
		sink.FlagExcludeDefault(
			sink.FlagUndoBufferSize,
			sink.FlagDevelopmentMode,
			sink.FlagNoopMode,
			sink.FlagProtoPath,
			sink.FlagProtoDescriptorSet,
			sink.FlagLiveBlockTimeDelta,
			sink.FlagFinalBlocksOnly,
		))

	estimateCmd.Flags().Float64("sample-percentage", 1, "Percentage of the requested range the endpoint samples to measure the output size")
	estimateCmd.Flags().BoolP("yes", "y", false, "Skip confirmation prompt and proceed with estimation")

	rootCmd.AddCommand(estimateCmd)
}

var estimateCmd = &cobra.Command{
	Use:   "estimate [<manifest> [<module_name>]]",
	Short: "Estimate Substreams usage costs, sampled by the endpoint",

	Long: cli.Dedent(`
		Estimates the cost (processed blocks and egress bytes) of running a Substreams module
		over a block range, by having the endpoint run a small sample of that range and
		extrapolating from it.

		The endpoint samples --sample-percentage of the range on its own workers, measures the
		size of the output it produced and reports back. The module data itself never leaves the
		endpoint, so the estimation costs processed blocks but no egress. The endpoint also knows
		what its cache already holds, so it reports how many blocks are actually left to process,
		and it handles modules with stores (whose segments cannot be run out of order: with stores, the
		sample is spread only over the part of the range the cache covers, or falls back to one
		contiguous run, which the report says).

		Against an endpoint that does not support this, use "substreams estimate-local", which
		samples from the client instead.

		Limitations and Warnings:
		  - Results are approximations based on sampling and can differ significantly from actual usage
		  - A non-representative sample (e.g., sampling only low-activity or high-activity periods)
		    can severely skew results

		This is a best-effort approximation tool. Actual costs may vary, sometimes greatly, from
		these estimates. Ensure that you use a representative sample range to improve accuracy of
		the estimates.
	`),
	RunE: estimateE,
	Args: cobra.RangeArgs(0, 2),
}

func estimateE(cmd *cobra.Command, args []string) error {
	logging.SetLevelFor(".*", zap.DPanicLevel, false)

	ctx := cmd.Context()
	cmd.SilenceUsage = true

	manifestPath, outputModule, err := ruiOrGuiManifestModulePositionalParams(args)
	if err != nil {
		return err
	}

	// Load auth environment file if it exists
	sink.LoadSubstreamsAuthEnvFile(manifestPath)

	// Parse flags to get sinker config
	sinkerConfig, err := sink.ConfigFromViper(cmd, sink.IgnoreOutputModuleType, manifestPath, outputModule, "sink/estimate", zlog, tracer)
	if err != nil {
		return err
	}
	sinkerConfig.Mode = sink.SubstreamsModeProduction

	return runRemoteEstimate(ctx, cmd, sinkerConfig, sflags.MustGetFloat64(cmd, "sample-percentage"))
}

func renderReportTable(report *ReportBuilder, headers []string, tableData [][]string) {
	if len(tableData) == 0 {
		return
	}

	firstRow := tableData[0]
	if len(firstRow) != len(headers) {
		panic(fmt.Errorf("first row length %d does not match headers length %d (headers %v, first row %v)",
			len(firstRow),
			len(headers),
			headers,
			firstRow,
		))
	}

	// Borderless, separator-less layout matching the previous rendering.
	settings := tw.Settings{Separators: tw.SeparatorsNone, Lines: tw.LinesNone}
	var rnd tw.Renderer
	if isTTY {
		// Cyan bold headers via the colorized renderer.
		rnd = renderer.NewColorized(renderer.ColorizedConfig{
			Borders:  tw.BorderNone,
			Settings: settings,
			Header:   renderer.Tint{FG: renderer.Colors{color.FgCyan, color.Bold}},
		})
	} else {
		rnd = renderer.NewBlueprint(tw.Rendition{Borders: tw.BorderNone, Settings: settings})
	}

	// Uppercase headers ourselves and disable auto-formatting: v1's auto-format
	// splits on punctuation (e.g. "Est. Blocks" -> "EST . BLOCKS").
	upperHeaders := make([]string, len(headers))
	for i, h := range headers {
		upperHeaders[i] = strings.ToUpper(h)
	}

	table := tablewriter.NewTable(report,
		tablewriter.WithRenderer(rnd),
		tablewriter.WithHeaderAutoFormat(tw.Off),
		tablewriter.WithHeaderAlignment(tw.AlignLeft),
		tablewriter.WithRowAlignment(tw.AlignLeft),
		tablewriter.WithHeaderAutoWrap(tw.WrapNone),
		tablewriter.WithRowAutoWrap(tw.WrapNone),
		tablewriter.WithPadding(tw.Padding{Right: "  "}),
	)
	table.Header(upperHeaders)
	_ = table.Bulk(tableData)
	_ = table.Render()
}

var _ io.Writer = (*ReportBuilder)(nil)

type ReportBuilder struct {
	writer io.Writer
}

func NewReportBuilder() *ReportBuilder {
	return &ReportBuilder{
		writer: os.Stdout,
	}
}

// Write implements io.Writer.
func (r *ReportBuilder) Write(p []byte) (n int, err error) {
	return r.writer.Write(p)
}

func (r *ReportBuilder) Line(format string, a ...any) {
	fmt.Fprintf(r.writer, format+"\n", a...)
}

func (r *ReportBuilder) LineStyled(style lipgloss.Style, format string, a ...any) {
	fmt.Fprintf(r.writer, "%s", style.Render(fmt.Sprintf(format, a...))+"\n")
}
