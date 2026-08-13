package tui

import (
	"fmt"
	"strings"

	"github.com/streamingfast/substreams/internal/formatx"
	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
)

// sessionContext carries what the preamble needs that the server does not send: those come
// from the command line and from the module graph resolved locally.
type sessionContext struct {
	Endpoint       string
	OutputModule   string
	ProductionMode bool
	Stages         int
}

// formatSessionPreamble renders the fixed header printed once per session.
//
// It is printed rather than rendered by the model on purpose: the live view is destroyed the
// moment the first block arrives, and the trace ID has to survive that — it is the one value
// a user must be able to quote when reporting a problem.
func formatSessionPreamble(session *pbsubstreamsrpc.SessionInit, sctx sessionContext) string {
	if session == nil {
		return ""
	}

	lines := []string{fmt.Sprintf("Session  %s", session.TraceId)}

	mode := "development"
	if sctx.ProductionMode {
		mode = "production"
	}
	stages := ""
	if sctx.Stages > 0 {
		stages = fmt.Sprintf("%d %s", sctx.Stages, pluralize(sctx.Stages, "stage"))
	}
	if module := formatx.JoinNonEmpty(" · ", sctx.OutputModule, mode, stages); module != "" {
		lines = append(lines, "  Module  "+module)
	}

	head := ""
	if session.ChainHead != 0 {
		head = "head " + formatx.Integer(session.ChainHead)
	}
	start := ""
	if session.ResolvedStartBlock != 0 {
		start = "start " + formatx.Integer(session.ResolvedStartBlock)
	}
	workers := ""
	if session.MaxParallelWorkers != 0 {
		workers = fmt.Sprintf("%d workers max", session.MaxParallelWorkers)
	}
	if chain := formatx.JoinNonEmpty(" · ", sctx.Endpoint, head, start, workers); chain != "" {
		lines = append(lines, "  Chain   "+chain)
	}

	if work := formatWork(session); work != "" {
		lines = append(lines, "  Work    "+work)
	}

	return strings.Join(lines, "\n")
}

func formatWork(session *pbsubstreamsrpc.SessionInit) string {
	return formatx.JoinNonEmpty(" · ",
		workPart("prepare stores", "stores", session.BlocksToProcessBeforeStartBlock, session.EffectiveBlocksToProcessBeforeStartBlock),
		workPart("in range", "range", session.BlocksToProcessAfterStartBlock, session.EffectiveBlocksToProcessAfterStartBlock),
	)
}

// workPart reads "2M prepare stores (1M cached)" while there is work left, and switches to
// "stores fully cached (6.1k)" once there is none — "0 prepare stores" is not an amount of
// work, it is the absence of it, and should not be phrased as a count.
func workPart(label, cachedLabel string, total, effective uint64) string {
	switch {
	case total == 0:
		return ""
	case effective == 0:
		return fmt.Sprintf("%s fully cached (%s)", cachedLabel, formatx.Count(total))
	case total > effective:
		return fmt.Sprintf("%s %s (%s cached)", formatx.Count(effective), label, formatx.Count(total-effective))
	default:
		return fmt.Sprintf("%s %s", formatx.Count(effective), label)
	}
}

// effectiveStageCount mirrors what the server actually does: outside production mode the
// module graph is executed as a single stage, whatever its depth suggests.
func effectiveStageCount(graphStages int, productionMode bool) int {
	if !productionMode {
		return 1
	}
	return graphStages
}

func pluralize(n int, word string) string {
	if n == 1 {
		return word
	}
	return word + "s"
}
