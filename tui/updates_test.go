package tui

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
)

func TestModelUpdate_SessionInit(t *testing.T) {
	m := model{StagesProgress: updatedRanges{}}

	// The `SessionInit` message is sent as-is through `tea.Program.Send`, it must be
	// handled by the model, otherwise the trace ID is never displayed.
	out, _ := m.Update(&pbsubstreamsrpc.SessionInit{
		TraceId:            "0123456789abcdef",
		ResolvedStartBlock: 1000,
	})

	updated, ok := out.(model)
	require.True(t, ok)

	assert.True(t, updated.Connected)
	assert.Equal(t, "0123456789abcdef", updated.TraceID)
	assert.Equal(t, uint64(1000), updated.BackprocessingCompleteAtBlock)

	assert.True(t, strings.Contains(updated.View(), "Connected (trace ID 0123456789abcdef)"), "view should show the trace ID, got:\n%s", updated.View())
}

func TestModelUpdate_ReconnectRefreshesTraceID(t *testing.T) {
	m := model{StagesProgress: updatedRanges{}}

	out, _ := m.Update(&pbsubstreamsrpc.SessionInit{TraceId: "first"})
	require.True(t, out.(model).Connected)

	// The sinker severed the stream and is about to retry, the previous trace ID is stale.
	out, _ = out.(model).Update(Connecting)
	assert.False(t, out.(model).Connected)
	assert.Empty(t, out.(model).TraceID)
	assert.True(t, strings.Contains(out.(model).View(), "Connecting..."))

	out, _ = out.(model).Update(&pbsubstreamsrpc.SessionInit{TraceId: "second"})
	assert.True(t, out.(model).Connected)
	assert.Equal(t, "second", out.(model).TraceID)
	assert.True(t, strings.Contains(out.(model).View(), "Connected (trace ID second)"))
}

// Rendering the connected view enables the stage progress section, and with it the bar
// mode, which was never reached before. Make sure neither can blow up the UI.
func TestModelView_ConnectedBarModeEdgeCases(t *testing.T) {
	progress := &pbsubstreamsrpc.ModulesProgress{
		Stages: []*pbsubstreamsrpc.Stage{
			{
				Modules:         []string{"map_token_balances"},
				CompletedRanges: []*pbsubstreamsrpc.BlockRange{{StartBlock: 25000000, EndBlock: 25652000}},
			},
		},
		RunningJobs: []*pbsubstreamsrpc.Job{
			{Stage: 0, StartBlock: 25652000, StopBlock: 25653000, DurationMs: 68000},
		},
	}

	cases := []struct {
		name               string
		barSize            uint64
		resolvedStartBlock uint64
	}{
		{"no window size message received yet", 0, 25652000},
		{"completed ranges past the backprocessing target", 40, 24000000},
		{"nominal", 40, 25652000},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := model{StagesProgress: updatedRanges{}, BarSize: c.barSize, BarMode: true}

			out, _ := m.Update(&pbsubstreamsrpc.SessionInit{TraceId: "abc", ResolvedStartBlock: c.resolvedStartBlock})
			out, _ = out.(model).Update(progress)

			view := out.(model).View()
			assert.True(t, strings.Contains(view, "Connected (trace ID abc)"), "got:\n%s", view)
			assert.False(t, strings.Contains(view, "failed rendering template"), "got:\n%s", view)
		})
	}
}
