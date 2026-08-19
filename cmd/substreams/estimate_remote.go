package main

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/olekukonko/tablewriter"
	"github.com/olekukonko/tablewriter/tw"
	"github.com/spf13/cobra"
	"github.com/streamingfast/cli"
	"github.com/streamingfast/cli/sflags"
	"github.com/streamingfast/substreams/client"
	"github.com/streamingfast/substreams/internal/formatx"
	pbsubstreamsrpcv4 "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v4"
	"github.com/streamingfast/substreams/sink"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// runRemoteEstimate asks the endpoint for an estimate instead of sampling from here.
//
// The endpoint runs the sample itself on its tier2 workers and measures the size of the
// output cache it produced, so no module data is ever transferred: the estimation costs
// processed blocks, never egress. It also knows what its cache already holds, which is
// something a client cannot see, and it handles module graphs with stores.
func runRemoteEstimate(ctx context.Context, cmd *cobra.Command, config *sink.SinkerConfig, samplePercentage float64) error {
	report := NewReportBuilder()

	// the endpoint resolves a negative start block and an open-ended stop block, so both are
	// only described here, never computed with
	startDescription := formatx.Integer(uint64(config.StartBlock))
	if config.StartBlock < 0 {
		startDescription = fmt.Sprintf("%d relative to chain head", config.StartBlock)
	}
	stopDescription := "chain head"
	if config.StopBlock != 0 {
		stopDescription = formatx.Integer(config.StopBlock)
	}

	report.Line(titleStyle.Render("Substreams Cost Estimation"))
	report.Line(dimStyle.Render(sectionSeparator))
	report.Line("%s %s", labelStyle.Render("Package:"), valueStyle.Render(config.Pkg.PackageMeta[0].Name))
	report.Line("%s  %s", labelStyle.Render("Module:"), valueStyle.Render(config.OutputModule.Name))
	report.Line("%s  %s", labelStyle.Render("Range:"), valueStyle.Render(fmt.Sprintf("%s - %s", startDescription, stopDescription)))
	report.Line("%s %s", labelStyle.Render("Samples:"), valueStyle.Render(fmt.Sprintf("%.2f%% of the range, sampled remotely", samplePercentage)))
	report.Line(dimStyle.Render(sectionSeparator))
	report.Line("")

	if !sflags.MustGetBool(cmd, "yes") {
		report.Line(labelStyle.Render("This estimation will:"))
		report.Line("  • Run %.2f%% of the requested range on the endpoint's workers", samplePercentage)
		report.Line("  • Consume those blocks from your plan's processed blocks quota")
		report.Line("  • Generate no egress: the data never leaves the endpoint")
		report.Line("")
		report.Line(noteStyle.Render("Results are approximations and may differ significantly from actual usage,"))
		report.Line(noteStyle.Render("especially if the sample is not representative (e.g., sampling only low-activity"))
		report.Line(noteStyle.Render("or high-activity periods)."))

		if !isTTY {
			report.Line(errorStyle.Render("Error: Terminal is not interactive. Use the -y or --yes flag to proceed without confirmation."))
			return fmt.Errorf("non-interactive terminal requires --yes flag")
		}

		report.Line("")
		confirmed, wasAnswered := cli.PromptConfirm("Do you want to proceed with this estimation?")
		if !wasAnswered || !confirmed {
			report.Line("")
			report.Line("Estimation cancelled.")
			return nil
		}
		report.Line("")
	}

	estimate, err := requestEstimate(ctx, config, samplePercentage)
	if err != nil {
		return err
	}

	generateRemoteReport(report, estimate)
	return nil
}

func requestEstimate(ctx context.Context, config *sink.SinkerConfig, samplePercentage float64) (*pbsubstreamsrpcv4.Estimate, error) {
	conn, closeFunc, callOpts, headers, err := client.NewSubstreamsClientConn(config.ClientConfig)
	if err != nil {
		return nil, fmt.Errorf("new substreams client connection: %w", err)
	}
	defer closeFunc()

	for key, value := range parseExtraHeaders(config.ExtraHeaders) {
		if headers == nil {
			headers = make(client.Headers)
		}
		headers[key] = value
	}
	if headers.IsSet() {
		ctx = metadata.AppendToOutgoingContext(ctx, headers.ToArray()...)
	}

	request := &pbsubstreamsrpcv4.EstimateRequest{
		Package:          config.Pkg,
		OutputModule:     config.OutputModule.Name,
		StartBlockNum:    config.StartBlock,
		StopBlockNum:     config.StopBlock,
		SamplePercentage: samplePercentage,
	}

	zlog.Info("requesting estimate",
		zap.String("output_module", request.OutputModule),
		zap.Int64("start_block", request.StartBlockNum),
		zap.Uint64("stop_block", request.StopBlockNum),
		zap.Float64("sample_percentage", request.SamplePercentage),
	)

	stream, err := pbsubstreamsrpcv4.NewEstimatorClient(conn).Estimate(ctx, request, callOpts...)
	if err != nil {
		return nil, estimatorCallError(err)
	}

	for {
		response, err := stream.Recv()
		if err == io.EOF {
			return nil, fmt.Errorf("endpoint closed the stream without sending an estimate")
		}
		if err != nil {
			return nil, estimatorCallError(err)
		}

		switch message := response.Message.(type) {
		case *pbsubstreamsrpcv4.EstimateResponse_Progress:
			fmt.Printf("Sampling: %d/%d segments\n", message.Progress.CompletedSegments, message.Progress.TotalSegments)
		case *pbsubstreamsrpcv4.EstimateResponse_Estimate:
			return message.Estimate, nil
		}
	}
}

func estimatorCallError(err error) error {
	if status.Code(err) == codes.Unimplemented {
		return fmt.Errorf("this endpoint does not support remote estimation, use `substreams estimate-local` to sample from the client instead: %w", err)
	}
	return err
}

func generateRemoteReport(report *ReportBuilder, estimate *pbsubstreamsrpcv4.Estimate) {
	sampling := estimate.Sampling

	report.LineStyled(dimStyle, "%s", sectionSeparator)
	report.LineStyled(headerStyle, "Segments")
	report.LineStyled(dimStyle, "%s", sectionSeparator)
	report.Line("")

	tableData := make([][]string, 0, len(sampling.Segments))
	for _, segment := range sampling.Segments {
		if segment.RepresentedBlocks == 0 {
			continue
		}
		tableData = append(tableData, []string{
			formatx.Integer(segment.RepresentedStartBlock),
			formatx.Integer(segment.RepresentedBlocks),
			formatx.Bytes(segment.EstimatedBytes),
		})
	}
	renderReportTable(report, []string{"Start", "Blocks", "Size"}, tableData,
		tablewriter.WithHeaderAlignment(tw.AlignRight),
		tablewriter.WithRowAlignment(tw.AlignRight))
	report.Line("")

	report.LineStyled(dimStyle, "%s", sectionSeparator)
	report.LineStyled(headerStyle, "Full Range Forecast")
	report.LineStyled(dimStyle, "%s", sectionSeparator)
	report.Line("")

	report.Line("%s %s",
		labelStyle.Render("Requested Range:"),
		valueStyle.Render(fmt.Sprintf("%s - %s (%s blocks)",
			formatx.Integer(estimate.ResolvedStartBlock),
			formatx.Integer(estimate.ResolvedStopBlock),
			formatx.Integer(estimate.ResolvedStopBlock-estimate.ResolvedStartBlock))))
	report.Line("%s %s",
		labelStyle.Render("Blocks Sampled:"),
		valueStyle.Render(fmt.Sprintf("%s (%.2f%% of range), %s messages",
			formatx.Integer(sampling.SampledBlocks),
			sampling.Percentage,
			formatx.Integer(sampling.SampledMessages))))
	report.Line("%s %s",
		labelStyle.Render("Stages:"),
		valueStyle.Render(fmt.Sprintf("%d (every stage processes every block)", estimate.StageCount)))
	report.Line("")

	// A block filter makes the module skip blocks its index rules out, so the count is
	// extrapolated from what the sample jobs ran on; without one it comes straight from the
	// request plan and is exact.
	blocksLabel := "Blocks To Process:"
	if estimate.BlockFiltered {
		blocksLabel = "Blocks To Process (estimated):"
	}
	report.Line("%s %s",
		labelStyle.Render(blocksLabel),
		successStyle.Render(formatx.Integer(estimate.BlocksToProcess)))
	report.Line("%s %s",
		labelStyle.Render("Blocks To Process (if nothing was cached):"),
		valueStyle.Render(formatx.Integer(estimate.TotalBlocksToProcessUncached)))
	report.Line("")

	report.Line("%s %s",
		labelStyle.Render("Estimated Egress Bytes (uncompressed):"),
		successStyle.Render(formatx.Bytes(estimate.EstimatedEgressBytes)))
	report.Line("")

	if sampling.Note != "" {
		report.LineStyled(noteStyle, "Sampling: %s", sampling.Note)
		report.Line("")
	}
	report.LineStyled(dimStyle, "%s", sectionSeparator)
}

func parseExtraHeaders(headers []string) map[string]string {
	out := make(map[string]string, len(headers))
	for _, header := range headers {
		key, value, found := strings.Cut(header, ":")
		if !found {
			continue
		}
		out[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}
	return out
}
