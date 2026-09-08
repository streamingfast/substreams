package tui

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/streamingfast/bstream"
	"github.com/streamingfast/substreams/internal/formatx"
	"github.com/streamingfast/substreams/pipeline/exec"
	"github.com/streamingfast/substreams/protodecode"
	"github.com/streamingfast/substreams/sink"
	"github.com/streamingfast/substreams/tools/test"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jhump/protoreflect/dynamic"
	"github.com/mattn/go-isatty"
	"github.com/streamingfast/shutter"
	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
	pbsubstreamsrpcv3 "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v3"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

//go:generate go-enum -f=$GOFILE --nocase --marshal --names

// ENUM(TUI, JSON, JSONL, CLOCK, CURSOR)
type OutputMode uint

type TUI struct {
	shutter *shutter.Shutter

	pkg     *pbsubstreams.Package
	decoder *protodecode.Decoder

	// Output mode flags
	isTerminal        bool
	outputMode        OutputMode
	prettyPrintOutput bool

	prog                    *tea.Program
	seenFirstData           bool
	RequiredProcessedBlocks uint64
	ResolvedStartBlock      uint64

	// endpoint is only known to the caller, the server does not report it, and it belongs in
	// the session preamble.
	endpoint string

	session *pbsubstreamsrpc.SessionInit

	interrupt   func()
	interrupted bool

	testRunner *test.Runner
}

// SetInterruptHandler registers what to do when the user hits Ctrl-C while the live view owns
// the terminal.
//
// Bubbletea puts the terminal in raw mode, so Ctrl-C arrives as a key event and the process
// never sees SIGINT — the command's signal handler does not run. Without this the UI would
// quit while the request kept streaming, and stopping for real took a second Ctrl-C, which
// only worked because the first one had released the terminal.
func (ui *TUI) SetInterruptHandler(f func()) { ui.interrupt = f }

// Interrupt is called by the live view when the user asks to stop. It cancels the work; the
// terminal is restored by the tea program shutting down, and the command's normal teardown
// prints the stats.
func (ui *TUI) Interrupt() {
	ui.interrupted = true
	fmt.Fprintln(os.Stderr, "Interrupted, shutting down…")

	if ui.interrupt != nil {
		ui.interrupt()
	}
}

// SessionWork reports the work the server said it had to do, in module-blocks: the total, and
// the part of it spent preparing the stores before the first block can be sent. ok is false
// until a session has been established.
func (ui *TUI) SessionWork() (total, prepareStores uint64, ok bool) {
	if ui.session == nil {
		return 0, 0, false
	}

	return ui.session.EffectiveBlocksToProcessBeforeStartBlock + ui.session.EffectiveBlocksToProcessAfterStartBlock,
		ui.session.EffectiveBlocksToProcessBeforeStartBlock,
		true
}

// SetEndpoint records the endpoint the request is sent to, for display purposes only.
func (ui *TUI) SetEndpoint(endpoint string) { ui.endpoint = endpoint }

func New(pkg *pbsubstreams.Package, outputStreamNames []string) (*TUI, error) {
	decoder, err := protodecode.NewDecoder(pkg, outputStreamNames)
	if err != nil {
		return nil, fmt.Errorf("new decoder: %w", err)
	}

	ui := &TUI{
		shutter: shutter.New(),
		pkg:     pkg,
		decoder: decoder,
	}

	return ui, nil
}

func (ui *TUI) SetTestRunner(testRunner *test.Runner) {
	ui.testRunner = testRunner
}

func (ui *TUI) Init(outputMode string, bytesRepresentation dynamic.BytesRepresentation) error {
	if err := ui.configureOutputMode(outputMode); err != nil {
		return err
	}

	dynamic.SetDefaultBytesRepresentation(bytesRepresentation)

	if ui.outputMode == OutputModeTUI {
		ui.ensureTerminalLocked()
	}

	return nil
}

func (ui *TUI) configureOutputMode(outputMode string) error {
	ui.isTerminal = isatty.IsTerminal(os.Stdout.Fd())

	if outputMode == "" {
		if ui.isTerminal {
			ui.outputMode = OutputModeTUI
		} else {
			ui.outputMode = OutputModeJSON
		}
	} else {
		var err error
		ui.outputMode, err = ParseOutputMode(outputMode)
		if err != nil {
			// Also accepts `ui` as an alias for `TUI`
			if outputMode == "UI" || outputMode == "ui" {
				ui.outputMode = OutputModeTUI
			} else {
				return fmt.Errorf("parse output mode: %w", err)
			}
		}
	}

	switch ui.outputMode {
	case OutputModeTUI:
		ui.prettyPrintOutput = true
	case OutputModeJSONL:
	case OutputModeCURSOR:
		fmt.Fprintln(os.Stderr, "printing cursor only, no data")
	case OutputModeCLOCK:
		fmt.Fprintln(os.Stderr, "Writing clock information only (no data)")
	case OutputModeJSON:
		ui.prettyPrintOutput = true
	default:
		panic(fmt.Errorf("unhandled output mode %q", ui.outputMode))
	}

	return nil
}

func (ui *TUI) Cancel() {
	if ui.prog == nil {
		return
	}
	err := ui.prog.ReleaseTerminal()
	if err != nil {
		err = fmt.Errorf("releasing terminal: %w", err)
	}

	if ui.testRunner != nil {
		ui.testRunner.LogResults()
	}

	ui.shutter.Shutdown(err)
}

func (ui *TUI) HandleBlockUndoSignal(ctx context.Context, undoSignal *pbsubstreamsrpc.BlockUndoSignal, cursor *sink.Cursor) error {
	switch ui.outputMode {
	case OutputModeTUI:
		printUndo(undoSignal.LastValidBlock, undoSignal.LastValidCursor)
		ui.ensureTerminalUnlocked()
	case OutputModeJSON, OutputModeJSONL:
		printUndoJSON(undoSignal.LastValidBlock, undoSignal.LastValidCursor)
	case OutputModeCLOCK:
		fmt.Println("UNDO:", undoSignal.LastValidBlock)
	case OutputModeCURSOR:
		fmt.Printf("UNDO: last valid:%d, cursor:%s\n", undoSignal.LastValidBlock.Number, cursor.String())
	}
	return nil
}

var debugSubstreamsRun = os.Getenv("SUBSTREAMS_DEBUG_RUN_SLOWDOWN") == "true"
var debugSubstreamsRunDelay = time.Millisecond * 100

func (ui *TUI) HandleBlockScopedData(ctx context.Context, data *pbsubstreamsrpc.BlockScopedData, isLive *bool, cursor *sink.Cursor) error {
	if debugSubstreamsRun {
		time.Sleep(debugSubstreamsRunDelay)
		debugSubstreamsRunDelay += time.Millisecond * 100
		if debugSubstreamsRunDelay > 500*time.Millisecond {
			debugSubstreamsRunDelay = 500 * time.Millisecond
		}
	}
	_ = isLive
	if ui.testRunner != nil {
		if err := ui.testRunner.Test(ctx, data.Output, data.DebugMapOutputs, data.DebugStoreOutputs, data.Clock); err != nil {
			return fmt.Errorf("test runner failed: %w", err)
		}
	}

	if data == nil {
		return nil
	}
	switch ui.outputMode {
	case OutputModeTUI:
		printClock(data)
	case OutputModeCLOCK:
		printClock(data)
		return nil
	case OutputModeCURSOR:
		fmt.Printf("BLOCK #%s (%s) age: %s cursor: %s\n", formatx.Integer(data.Clock.Number), data.Clock.Id, time.Since(data.Clock.Timestamp.AsTime()), cursor.String())
		return nil
	}

	ui.seenFirstData = true
	if ui.outputMode == OutputModeTUI {
		ui.ensureTerminalUnlocked()
		return ui.decoratedBlockScopedData(data.Output, data.DebugMapOutputs, data.DebugStoreOutputs, data.Clock, data.PartialIndex, data.IsLastPartial)
	} else {
		return ui.jsonBlockScopedData(data.Output, data.DebugMapOutputs, data.DebugStoreOutputs, data.Clock, data.PartialIndex, data.IsLastPartial)
	}
}

func (ui *TUI) HandleProgress(ctx context.Context, progress *pbsubstreamsrpc.ModulesProgress) {
	if !ui.seenFirstData {
		if ui.outputMode == OutputModeTUI {
			ui.ensureTerminalLocked()
			ui.prog.Send(progress)
		}
	}
}

func (ui *TUI) HandleDebugSnapshotData(ctx context.Context, debug *pbsubstreamsrpc.InitialSnapshotData) error {
	if ui.outputMode == OutputModeTUI {
		ui.ensureTerminalUnlocked()
		return ui.decoratedSnapshotData(debug)
	} else {
		return ui.jsonSnapshotData(debug)
	}
}

func (ui *TUI) HandleDebugSnapshotComplete(ctx context.Context, complete *pbsubstreamsrpc.InitialSnapshotComplete) error {
	_ = complete
	if ui.outputMode == OutputModeTUI {
		fmt.Println("Snapshot data dump complete")
	}
	return nil
}

func (ui *TUI) HandleSession(ctx context.Context, req *pbsubstreamsrpcv3.Request, session *pbsubstreamsrpc.SessionInit) error {
	if session.BlocksToProcessAfterStartBlock != 0 {
		ui.RequiredProcessedBlocks = session.EffectiveBlocksToProcessBeforeStartBlock + session.EffectiveBlocksToProcessAfterStartBlock
	}
	ui.ResolvedStartBlock = session.ResolvedStartBlock
	ui.session = session

	execGraph, err := exec.NewOutputModuleGraph(req.OutputModule, req.ProductionMode, req.Package.GetModules(), bstream.GetProtocolFirstStreamableBlock)
	if err != nil {
		return fmt.Errorf("cannot handle module graph: %w", err)
	}

	if ui.outputMode == OutputModeTUI {
		// Printed rather than rendered by the model: the live view is torn down as soon as the
		// first block arrives, and the trace ID has to outlive it.
		//
		// The live region is stopped for the duration of the write instead of going through
		// prog.Println, which only repaints on the next render tick — a request whose blocks are
		// entirely cached tears the program down before that tick ever comes, and the preamble
		// then lands after the first block of data.
		ui.ensureTerminalUnlocked()

		// Trailing blank line: the progress block, or an error, is drawn immediately below and
		// otherwise runs straight into the session header.
		fmt.Fprintf(os.Stderr, "%s\n\n", formatSessionPreamble(session, sessionContext{
			Endpoint:       ui.endpoint,
			OutputModule:   req.OutputModule,
			ProductionMode: req.ProductionMode,
			Stages:         effectiveStageCount(len(execGraph.StagedUsedModules()), req.ProductionMode),
		}))

		ui.ensureTerminalLocked()
		ui.prog.Send(session)
	} else {
		fmt.Fprintf(os.Stderr, "TraceID: %s\n", session.TraceId)
		if session.ChainHead != 0 {
			fmt.Fprintf(os.Stderr, "Server HEAD block: %d\n", session.ChainHead)
		}
		stages := len(execGraph.StagedUsedModules())
		if stages == 1 || !req.ProductionMode {
			fmt.Fprintln(os.Stderr, "This request will be processed in a single stage")
		} else {
			fmt.Fprintf(os.Stderr, "This request will be processed in %d stages\n", stages)
		}

		if session.BlocksToProcessBeforeStartBlock != 0 {
			stageCount := fmt.Sprintf("%d stage", stages-1)
			if stages > 2 {
				stageCount += "s"
			}
			fmt.Fprintf(os.Stderr, "Blocks to process to prepare the stores in %s: %d (%d already cached)\n", stageCount, session.EffectiveBlocksToProcessBeforeStartBlock, session.BlocksToProcessBeforeStartBlock-session.EffectiveBlocksToProcessBeforeStartBlock)
		}

		if session.BlocksToProcessAfterStartBlock != 0 {
			fmt.Fprintf(os.Stderr, "Blocks to process in requested range: %d (%d already cached)\n", session.EffectiveBlocksToProcessAfterStartBlock, session.BlocksToProcessAfterStartBlock-session.EffectiveBlocksToProcessAfterStartBlock)
		}
	}
	return nil
}

// HandleError is called by the sinker when the stream is severed and about to be retried. The
// session that follows carries a brand new trace ID, so the UI must stop advertising the previous
// one until we are effectively re-connected.
func (ui *TUI) HandleError(ctx context.Context, err error) {
	_ = ctx
	_ = err

	// Cancelling the request makes the stream fail, and the sinker reports that failure the same
	// way it reports a severed connection. Repainting "Connecting..." while tearing down would
	// claim the opposite of what is happening.
	if ui.outputMode == OutputModeTUI && !ui.interrupted {
		ui.Connecting()
	}
}

func (ui *TUI) ensureTerminalUnlocked() {
	if ui.prog == nil {
		return
	}
	ui.prog.ReleaseTerminal()
	ui.prog.Kill()
	ui.prog = nil
	time.Sleep(10 * time.Millisecond)
}

func (ui *TUI) ensureTerminalLocked() {
	if ui.prog != nil {
		return
	}

	ui.prog = tea.NewProgram(newModel(ui))
	go func() {
		if _, err := ui.prog.Run(); err != nil {
			if strings.Contains(err.Error(), context.Canceled.Error()) {
				// Weird case where we need to check the error string message to know the underlying error
				// The error returned is a fmt.wrapError which contains the context.Canceled error
				return
			}
			if err != tea.ErrProgramKilled {
				// tea library handles the error weirdly. It will return  an ErrProgramKilled when
				// the context has been canceled. This occurs when the program shutdowns, which should not
				// actually be an error
				fmt.Printf("Failed bubble tea program: %s %T\n", err, err)
			}
		}
	}()
}

// AbortProgress closes the backprocessing block when the request ended before a single block
// came out of it.
//
// The live region renders inline, so every frame it drew stays in the scrollback. Without
// this the last thing the user reads above the error is "Backprocessing  starting…", which
// reads as output that got cut off rather than as a request that never got going.
func (ui *TUI) AbortProgress() {
	if ui.outputMode != OutputModeTUI || ui.seenFirstData {
		return
	}

	ui.ensureTerminalUnlocked()
	fmt.Fprintln(os.Stderr, "Backprocessing  aborted")
}

func (ui *TUI) CleanUpTerminal() {
	if ui.prog != nil {
		if err := ui.prog.ReleaseTerminal(); err != nil {
			fmt.Println("failed releasing terminal:", err)
		}
	}
}

func (ui *TUI) OnTerminated(f func(error)) {
	ui.shutter.OnTerminated(f)
}
