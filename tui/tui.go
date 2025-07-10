package tui

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/bobg/go-generics/v3/slices"
	"github.com/streamingfast/substreams/sink"
	"github.com/streamingfast/substreams/tui2/common"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jhump/protoreflect/desc"
	"github.com/jhump/protoreflect/dynamic"
	"github.com/mattn/go-isatty"
	"github.com/streamingfast/shutter"
	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

//go:generate go-enum -f=$GOFILE --nocase --marshal --names

// ENUM(TUI, JSON, JSONL, CLOCK, CURSOR)
type OutputMode uint

type OutputStreamPattern struct {
	pattern string
	regex   *regexp.Regexp
}

func NewOutputStreamPattern(pattern string) OutputStreamPattern {
	regex, err := regexp.Compile(pattern)
	if err != nil {
		return OutputStreamPattern{pattern: pattern, regex: nil}
	}

	return OutputStreamPattern{pattern: pattern, regex: regex}
}

func (o *OutputStreamPattern) Matches(input string) bool {
	if o.regex == nil {
		return o.pattern == input
	}

	return o.regex.MatchString(input)
}

type TUI struct {
	shutter *shutter.Shutter

	endpoint          string
	Req               *pbsubstreamsrpc.Request
	pkg               *pbsubstreams.Package
	outputStreamNames []OutputStreamPattern

	// Output mode flags
	isTerminal        bool
	outputMode        OutputMode
	prettyPrintOutput bool

	prog                    *tea.Program
	seenFirstData           bool
	TotalReadBytes          uint64
	TotalProcessedBlocks    uint64
	RequiredProcessedBlocks uint64
	ResolvedStartBlock      uint64

	msgDescs       map[string]*desc.MessageDescriptor
	decodeMsgTypes map[string]func(in []byte) string
	msgTypes       map[string]string // Replace by calls to GetFullyQualifiedName() on the `msgDescs`
	anyResolver    *pbsubstreams.PackageAnyResolver
}

func New(endpoint string, req *pbsubstreamsrpc.Request, pkg *pbsubstreams.Package, outputStreamNames []string) (*TUI, error) {
	anyResolver, err := pkg.NewAnyResolver()
	if err != nil {
		return nil, fmt.Errorf("new any resolver: %w", err)
	}

	ui := &TUI{
		shutter:           shutter.New(),
		endpoint:          endpoint,
		Req:               req,
		pkg:               pkg,
		outputStreamNames: slices.Map(outputStreamNames, func(s string) OutputStreamPattern { return NewOutputStreamPattern(s) }),
		decodeMsgTypes:    map[string]func(in []byte) string{},
		msgTypes:          map[string]string{},
		msgDescs:          map[string]*desc.MessageDescriptor{},
		anyResolver:       anyResolver,
	}

	return ui, nil
}

func (ui *TUI) Init(outputMode string) error {
	if err := ui.configureOutputMode(outputMode); err != nil {
		return err
	}

	if ui.outputMode == OutputModeTUI {
		ui.ensureTerminalLocked()
	}

	fileDescs, err := desc.CreateFileDescriptors(ui.pkg.ProtoFiles)
	if err != nil {
		return fmt.Errorf("couldn't convert, should do this check much earlier: %w", err)
	}

	for _, mod := range ui.pkg.Modules.Modules {
		for _, outputStreamName := range ui.outputStreamNames {
			if outputStreamName.Matches(mod.Name) {
				var msgType string
				switch modKind := mod.Kind.(type) {
				case *pbsubstreams.Module_KindStore_:
					msgType = modKind.KindStore.ValueType
				case *pbsubstreams.Module_KindMap_:
					msgType = modKind.KindMap.OutputType
				case *pbsubstreams.Module_KindBlockIndex_:
					msgType = modKind.KindBlockIndex.OutputType
				}

				msgType = strings.TrimPrefix(msgType, "proto:")

				ui.msgTypes[mod.Name] = msgType

				var msgDesc *desc.MessageDescriptor
				for _, file := range fileDescs {
					msgDesc = file.FindMessage(msgType)
					if msgDesc != nil {
						break
					}
				}
				ui.msgDescs[mod.Name] = msgDesc
			}
		}
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

	dynamic.SetDefaultBytesRepresentation(common.InferBytesRepresentation(ui.pkg.Network, ui.endpoint))

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
		fmt.Println(cursor.String())
	}
	return nil
}

func (ui *TUI) HandleBlockScopedData(ctx context.Context, data *pbsubstreamsrpc.BlockScopedData, isLive *bool, cursor *sink.Cursor) error {
	_ = isLive

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
		fmt.Println(cursor.String())
		return nil
	}

	ui.seenFirstData = true
	if ui.outputMode == OutputModeTUI {
		ui.ensureTerminalUnlocked()
		return ui.decoratedBlockScopedData(data.Output, data.DebugMapOutputs, data.DebugStoreOutputs, data.Clock)
	} else {
		return ui.jsonBlockScopedData(data.Output, data.DebugMapOutputs, data.DebugStoreOutputs, data.Clock)
	}
}

func (ui *TUI) HandleProgress(ctx context.Context, progress *pbsubstreamsrpc.ModulesProgress) {
	if progress.ProcessedBytes != nil {
		ui.TotalReadBytes = progress.ProcessedBytes.TotalBytesRead
	}
	ui.TotalProcessedBlocks = progress.ProcessedBlocks

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

func (ui *TUI) HandleSession(ctx context.Context, session *pbsubstreamsrpc.SessionInit) error {
	if session.BlocksToProcessAfterStartBlock != 0 {
		ui.RequiredProcessedBlocks = session.EffectiveBlocksToProcessBeforeStartBlock + session.EffectiveBlocksToProcessAfterStartBlock
	}
	ui.ResolvedStartBlock = session.ResolvedStartBlock

	if ui.outputMode == OutputModeTUI {
		ui.ensureTerminalLocked()
		ui.prog.Send(session)
	}
	//	} else {
	//		execGraph, err := exec.NewOutputModuleGraph(ui.Req.OutputModule, ui.Req.ProductionMode, ui.Req.Modules, bstream.GetProtocolFirstStreamableBlock//)
	//		if err != nil {
	//			return fmt.Errorf("cannot handle module graph: %w", err)
	//		}
	//
	//		fmt.Fprintf(os.Stderr, "TraceID: %s\n", session.TraceId)
	//		if session.ChainHead != 0 {
	//			fmt.Fprintf(os.Stderr, "Server HEAD block: %d\n", session.ChainHead)
	//		}
	//		stages := len(execGraph.StagedUsedModules())
	//		if stages == 1 || !ui.Req.ProductionMode {
	//			fmt.Fprintln(os.Stderr, "This request will be processed in a single stage")
	//		} else {
	//			fmt.Fprintf(os.Stderr, "This request will be processed in %d stages\n", stages)
	//		}
	//
	//		if session.BlocksToProcessBeforeStartBlock != 0 {
	//			stageCount := fmt.Sprintf("%d stage", stages-1)
	//			if stages > 2 {
	//				stageCount += "s"
	//			}
	//			fmt.Fprintf(os.Stderr, "Blocks to process to prepare the stores in %s: %d (%d already cached)\n", stageCount, //session.EffectiveBlocksToProcessBeforeStartBlock, session.BlocksToProcessBeforeStartBlock-session.EffectiveBlocksToProcessBeforeStartBlock)
	//		}
	//
	//		if session.BlocksToProcessAfterStartBlock != 0 {
	//			fmt.Fprintf(os.Stderr, "Blocks to process in requested range: %d (%d already cached)\n", session.EffectiveBlocksToProcessAfterStartBlock, //session.BlocksToProcessAfterStartBlock-session.EffectiveBlocksToProcessAfterStartBlock)
	//		}
	//	}
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
