package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
)

// testClock hands out a deterministic, manually advanced time so that windowed rates, ETAs
// and job ages never depend on how fast the test machine is.
type testClock struct{ at time.Time }

func newTestClock() *testClock {
	return &testClock{at: time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)}
}

func (c *testClock) now() time.Time          { return c.at }
func (c *testClock) advance(d time.Duration) { c.at = c.at.Add(d) }

func newTestModel(clock *testClock) model {
	m := model{now: clock.now}
	return m.withDefaults()
}

func update(t *testing.T, m model, msgs ...any) model {
	t.Helper()

	for _, msg := range msgs {
		out, _ := m.Update(msg)
		next, ok := out.(model)
		require.True(t, ok)
		m = next
	}
	return m
}

func TestModelUpdate_SessionInit(t *testing.T) {
	m := update(t, newTestModel(newTestClock()), &pbsubstreamsrpc.SessionInit{
		TraceId:            "0123456789abcdef",
		ResolvedStartBlock: 1000,
	})

	assert.True(t, m.Connected)
	assert.Equal(t, "0123456789abcdef", m.TraceID)
	require.NotNil(t, m.session)
	assert.Equal(t, uint64(1000), m.session.ResolvedStartBlock)
}

func TestModelUpdate_ReconnectResetsSessionState(t *testing.T) {
	clock := newTestClock()

	m := update(t, newTestModel(clock),
		&pbsubstreamsrpc.SessionInit{TraceId: "first", EffectiveBlocksToProcessAfterStartBlock: 1000},
		&pbsubstreamsrpc.ModulesProgress{ProcessedBlocks: 400},
	)
	require.NotNil(t, m.progress)

	// The sinker severed the stream and is about to retry: everything the previous session
	// reported is stale. Keeping the rate samples would show the discontinuity as a burst.
	m = update(t, m, Connecting)
	assert.False(t, m.Connected)
	assert.Empty(t, m.TraceID)
	assert.Nil(t, m.session)
	assert.Nil(t, m.progress)
	assert.Empty(t, m.globalRate.samples)
	assert.Empty(t, m.moduleRates.samples)
	assert.Equal(t, "Connecting...\n", m.View())

	m = update(t, m, &pbsubstreamsrpc.SessionInit{TraceId: "second"})
	assert.True(t, m.Connected)
	assert.Equal(t, "second", m.TraceID)
}

func TestModelUpdate_ProgressFeedsWindows(t *testing.T) {
	clock := newTestClock()

	m := update(t, newTestModel(clock), &pbsubstreamsrpc.SessionInit{
		TraceId:                                 "abc",
		EffectiveBlocksToProcessAfterStartBlock: 10_000,
	})

	m = update(t, m, &pbsubstreamsrpc.ModulesProgress{
		ProcessedBlocks: 1_000,
		RunningJobs:     []*pbsubstreamsrpc.Job{{Stage: 0, ProgressBlocks: 500}},
		ModulesStats:    []*pbsubstreamsrpc.ModuleStats{{Name: "map_a", TotalProcessingTimeMs: 1_000, TotalProcessedBlockCount: 100}},
	})

	clock.advance(20 * time.Second)

	m = update(t, m, &pbsubstreamsrpc.ModulesProgress{
		ProcessedBlocks: 3_000,
		RunningJobs:     []*pbsubstreamsrpc.Job{{Stage: 0, ProgressBlocks: 500}},
		ModulesStats:    []*pbsubstreamsrpc.ModuleStats{{Name: "map_a", TotalProcessingTimeMs: 3_400, TotalProcessedBlockCount: 300}},
	})

	// 2000 module-blocks over 20 seconds.
	rate, span, ok := m.globalRate.perSecond()
	require.True(t, ok)
	assert.Equal(t, 20*time.Second, span)
	assert.InDelta(t, 100.0, rate, 0.001)

	// 2400ms of extra processing over 200 extra blocks, against a 14.67ms lifetime average.
	cost, ok := m.moduleRates.msPerBlock("map_a")
	require.True(t, ok)
	assert.InDelta(t, 12.0, cost, 0.001)
}

// A window older than its duration must keep covering exactly that duration, which matters
// because the server stretches its progress interval to 5s on long requests.
func TestRateWindow_TrimsToWindow(t *testing.T) {
	clock := newTestClock()
	w := newRateWindow(30 * time.Second)

	for i := range 20 {
		w.add(clock.now(), uint64(i*100))
		clock.advance(5 * time.Second)
	}

	_, span, ok := w.perSecond()
	require.True(t, ok)
	assert.LessOrEqual(t, span, 35*time.Second)
	assert.GreaterOrEqual(t, span, 30*time.Second)
}

func TestRateWindow_IgnoresCounterReset(t *testing.T) {
	clock := newTestClock()
	w := newRateWindow(30 * time.Second)

	w.add(clock.now(), 5_000)
	clock.advance(5 * time.Second)
	w.add(clock.now(), 100)

	_, _, ok := w.perSecond()
	assert.False(t, ok)
}

func TestModelView_ConnectedEdgeCases(t *testing.T) {
	progress := &pbsubstreamsrpc.ModulesProgress{
		ProcessedBlocks: 652_000,
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
		width              int
		resolvedStartBlock uint64
	}{
		{"no window size message received yet", 0, 25652000},
		{"completed ranges past the backprocessing target", 40, 24000000},
		{"nominal", 120, 25652000},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := newTestModel(newTestClock())
			m.Width = c.width

			m = update(t, m,
				&pbsubstreamsrpc.SessionInit{
					TraceId:                                 "abc",
					ResolvedStartBlock:                      c.resolvedStartBlock,
					LinearHandoffBlock:                      c.resolvedStartBlock,
					EffectiveBlocksToProcessAfterStartBlock: 1_000_000,
				},
				progress,
			)

			view := m.View()
			assert.True(t, strings.HasPrefix(view, "Backprocessing"), "got:\n%s", view)
			assert.False(t, strings.Contains(view, "NaN"), "got:\n%s", view)
			assert.False(t, strings.Contains(view, "%!"), "got:\n%s", view)
		})
	}
}

// Bubbletea holds the terminal in raw mode, so Ctrl-C reaches the model as a key rather than
// as SIGINT. If the key does not cancel the work, the UI quits while the request keeps
// streaming and it takes a second Ctrl-C to actually stop.
func TestModelUpdate_CtrlCCancelsTheWork(t *testing.T) {
	for _, key := range []tea.KeyType{tea.KeyCtrlC, tea.KeyCtrlBackslash} {
		interrupted := false

		ui := &TUI{}
		ui.SetInterruptHandler(func() { interrupted = true })

		m := newTestModel(newTestClock())
		m.ui = ui

		_, cmd := m.Update(tea.KeyMsg{Type: key})

		assert.True(t, interrupted, "key %v must cancel the work", key)
		require.NotNil(t, cmd, "key %v must also quit the program", key)
		assert.Equal(t, tea.Quit(), cmd())
	}
}

func TestComputeStageStats_MeasuresSquashedReadinessNotProducedRanges(t *testing.T) {
	// Every segment produced, only a third squashed. Measured from CompletedRanges this reads
	// 100%; the honest figure is the squashed frontier.
	stats := computeStageStats([]*pbsubstreamsrpc.Stage{{
		CompletedRanges:        []*pbsubstreamsrpc.BlockRange{{StartBlock: 1_000_000, EndBlock: 2_000_000}},
		ReadyUpToExclusive:     1_300_000,
		SquashWaitSegmentCount: 35,
	}}, nil, 2_000_000)

	require.Len(t, stats, 1)
	assert.InDelta(t, 0.3, stats[0].Ratio, 0.001)
	assert.Equal(t, uint64(35), stats[0].SquashWait)
}

// A stage that has not started reports where its modules begin, which is what anchors the low
// end of its bar — ranges and jobs say nothing at all at that point.
func TestComputeStageStats_UnstartedStageAnchorsOnReadiness(t *testing.T) {
	stats := computeStageStats([]*pbsubstreamsrpc.Stage{{ReadyUpToExclusive: 1_000_000}}, nil, 2_000_000)

	require.Len(t, stats, 1)
	assert.Zero(t, stats[0].Ratio)
	assert.Zero(t, stats[0].SquashWait)
}

// Zero is a legitimate readiness value for a stage whose modules and chain both start at 0, so
// it must render as 0% rather than being taken for "no data" and skipped.
func TestComputeStageStats_ZeroReadinessIsAValue(t *testing.T) {
	stats := computeStageStats([]*pbsubstreamsrpc.Stage{{
		ReadyUpToExclusive: 0,
		CompletedRanges:    []*pbsubstreamsrpc.BlockRange{{StartBlock: 0, EndBlock: 500}},
	}}, nil, 1_000)

	require.Len(t, stats, 1)
	assert.Zero(t, stats[0].Ratio)
}

func TestModelView_FailureBlock(t *testing.T) {
	m := newTestModel(newTestClock())
	m.Failures = 2
	m.LastFailure = &pbsubstreamsrpc.Error{
		Reason:        "module panicked",
		Logs:          []string{"first", "second"},
		LogsTruncated: true,
	}

	m = update(t, m, &pbsubstreamsrpc.SessionInit{TraceId: "abc"})

	view := m.View()
	assert.Contains(t, view, "Failures: 2.")
	assert.Contains(t, view, "  Reason: module panicked")
	assert.Contains(t, view, "    first")
	assert.Contains(t, view, "  <logs truncated>")
}
