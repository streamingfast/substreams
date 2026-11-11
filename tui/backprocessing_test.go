package tui

import (
	"io"
	"os"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func captureStderr(t *testing.T, f func()) string {
	t.Helper()

	r, w, err := os.Pipe()
	require.NoError(t, err)

	stderr := os.Stderr
	os.Stderr = w
	defer func() { os.Stderr = stderr }()

	f()
	require.NoError(t, w.Close())

	out, err := io.ReadAll(r)
	require.NoError(t, err)
	return string(out)
}

func newBackprocessingTUI() *TUI {
	return &TUI{outputMode: OutputModeTUI, session: nominalSession(), sessionAt: time.Now()}
}

func TestTrackBackprocessing_CompletesOnALaterBlock(t *testing.T) {
	ui := newBackprocessingTUI()

	out := captureStderr(t, func() { ui.trackBackprocessing(backprocessingTarget - 1) })
	assert.Empty(t, out, "a block produced by tier2")

	out = captureStderr(t, func() { ui.trackBackprocessing(backprocessingTarget) })
	assert.True(t, strings.HasPrefix(out, "Backprocessing  complete"), out)

	out = captureStderr(t, func() {
		ui.trackBackprocessing(backprocessingTarget + 1)
		ui.CompleteProgress()
	})
	assert.Empty(t, out, "reported once")
}

func TestCompleteProgress_StreamEndedBeforeTheHandoff(t *testing.T) {
	ui := newBackprocessingTUI()

	out := captureStderr(t, func() { ui.CompleteProgress() })
	assert.True(t, strings.HasPrefix(out, "Backprocessing  complete"), out)
}

func TestCompleteProgress_NothingToBackprocess(t *testing.T) {
	ui := newBackprocessingTUI()
	ui.session = &pbsubstreamsrpc.SessionInit{LinearHandoffBlock: backprocessingTarget}

	out := captureStderr(t, func() { ui.CompleteProgress() })
	assert.Empty(t, out)
}

// The user can quit the live view while the request keeps streaming.
func TestTrackBackprocessing_LiveViewAlreadyQuit(t *testing.T) {
	ui := newBackprocessingTUI()

	exited := make(chan struct{})
	ui.prog = tea.NewProgram(newTestModel(newTestClock()), tea.WithInput(nil), tea.WithOutput(io.Discard))
	ui.progExited = exited
	go func() {
		defer close(exited)
		_, _ = ui.prog.Run()
	}()
	ui.prog.Quit()
	<-exited

	out := captureStderr(t, func() { ui.trackBackprocessing(backprocessingTarget) })
	assert.True(t, strings.HasPrefix(out, "Backprocessing  complete"), out)
	assert.Nil(t, ui.prog)
}
