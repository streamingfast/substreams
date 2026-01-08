package tui

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/streamingfast/bstream"
	"github.com/streamingfast/substreams/pipeline/exec"
	"github.com/streamingfast/substreams/protodecode"
	"github.com/streamingfast/substreams/sink"
	"github.com/streamingfast/substreams/tools/test"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jhump/protoreflect/dynamic"
	"github.com/mattn/go-isatty"
	"github.com/streamingfast/shutter"
	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
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

	testRunner *test.Runner
}

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
		fmt.Println("printing cursor only, no data")
	case OutputModeCLOCK:
		fmt.Println("Writing clock information only (no data)")
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
		fmt.Printf("BLOCK #%s (%s) age: %s cursor: %s\n", humanize.Comma(int64(data.Clock.Number)), data.Clock.Id, time.Since(data.Clock.Timestamp.AsTime()), cursor.String())
		return nil
	}

	ui.seenFirstData = true
	if ui.outputMode == OutputModeTUI {
		ui.ensureTerminalUnlocked()
		return ui.decoratedBlockScopedData(data.Output, data.DebugMapOutputs, data.DebugStoreOutputs, data.Clock, 0)
	} else {
		return ui.jsonBlockScopedData(data.Output, data.DebugMapOutputs, data.DebugStoreOutputs, data.Clock, 0)
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

func (ui *TUI) HandleSession(ctx context.Context, req *pbsubstreamsrpc.Request, session *pbsubstreamsrpc.SessionInit) error {
	if session.BlocksToProcessAfterStartBlock != 0 {
		ui.RequiredProcessedBlocks = session.EffectiveBlocksToProcessBeforeStartBlock + session.EffectiveBlocksToProcessAfterStartBlock
	}
	ui.ResolvedStartBlock = session.ResolvedStartBlock

	if ui.outputMode == OutputModeTUI {
		ui.ensureTerminalLocked()
		ui.prog.Send(session)
	} else {
		execGraph, err := exec.NewOutputModuleGraph(req.OutputModule, req.ProductionMode, req.Modules, bstream.GetProtocolFirstStreamableBlock)
		if err != nil {
			return fmt.Errorf("cannot handle module graph: %w", err)
		}

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
