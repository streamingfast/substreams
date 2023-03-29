package tui

import (
	"fmt"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jhump/protoreflect/desc"
	"github.com/mattn/go-isatty"
	"github.com/streamingfast/shutter"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

//go:generate go-enum -f=$GOFILE --nocase --marshal --names

// ENUM(TUI, JSON, JSONL)
type OutputMode uint

type TUI struct {
	shutter *shutter.Shutter

	req               *pbsubstreams.Request
	pkg               *pbsubstreams.Package
	outputStreamNames []string

	// Output mode flags
	isTerminal        bool
	outputMode        OutputMode
	prettyPrintOutput bool

	prog          *tea.Program
	seenFirstData bool

	msgDescs       map[string]*desc.MessageDescriptor
	decodeMsgTypes map[string]func(in []byte) string
	msgTypes       map[string]string // Replace by calls to GetFullyQualifiedName() on the `msgDescs`
}

func New(req *pbsubstreams.Request, pkg *pbsubstreams.Package, outputStreamNames []string) *TUI {
	ui := &TUI{
		shutter:           shutter.New(),
		req:               req,
		pkg:               pkg,
		outputStreamNames: outputStreamNames,
		decodeMsgTypes:    map[string]func(in []byte) string{},
		msgTypes:          map[string]string{},
		msgDescs:          map[string]*desc.MessageDescriptor{},
	}

	return ui
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
			if mod.Name == outputStreamName {
				var msgType string
				switch modKind := mod.Kind.(type) {
				case *pbsubstreams.Module_KindStore_:
					msgType = modKind.KindStore.ValueType
				case *pbsubstreams.Module_KindMap_:
					msgType = modKind.KindMap.OutputType
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
			return fmt.Errorf("parse output mode: %w", err)
		}
	}

	switch ui.outputMode {
	case OutputModeTUI:
		ui.prettyPrintOutput = true
	case OutputModeJSONL:
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

	ui.shutter.Shutdown(err)
}

func (ui *TUI) filterOutputModules(in []*pbsubstreams.ModuleOutput) (out []*pbsubstreams.ModuleOutput) {
	for _, mo := range in {
		if _, ok := ui.msgTypes[mo.Name]; ok {
			out = append(out, mo)
		}
	}

	return
}

func (ui *TUI) IncomingMessage(resp *pbsubstreams.Response) error {
	switch m := resp.Message.(type) {
	case *pbsubstreams.Response_Data:
		if ui.outputMode == OutputModeTUI {
			printClock(m.Data)
		}
		if m.Data == nil {
			return nil
		}
		outputs := ui.filterOutputModules(m.Data.Outputs)
		if len(outputs) == 0 {
			return nil
		}
		ui.seenFirstData = true
		if ui.outputMode == OutputModeTUI {
			ui.ensureTerminalUnlocked()
			return ui.decoratedBlockScopedData(outputs, m.Data.Clock)
		} else {
			return ui.jsonBlockScopedData(outputs, m.Data.Clock)
		}
	case *pbsubstreams.Response_Progress:
		if ui.seenFirstData {
			ui.formatPostDataProgress(m)
		} else {
			if ui.outputMode == OutputModeTUI {
				ui.ensureTerminalLocked()
				for _, module := range m.Progress.Modules {
					ui.prog.Send(module)
				}
			}
		}
	case *pbsubstreams.Response_DebugSnapshotData:
		if ui.outputMode == OutputModeTUI {
			ui.ensureTerminalUnlocked()
			return ui.decoratedSnapshotData(m.DebugSnapshotData)
		} else {
			return ui.jsonSnapshotData(m.DebugSnapshotData)
		}

	case *pbsubstreams.Response_DebugSnapshotComplete:
		if ui.outputMode == OutputModeTUI {
			fmt.Println("Snapshot data dump complete")
		}

	case *pbsubstreams.Response_Session:
		if ui.outputMode == OutputModeTUI {
			ui.ensureTerminalLocked()
			ui.prog.Send(m)
		} else {
			fmt.Printf("TraceID: %s\n", m.Session.TraceId)
		}

	default:
		fmt.Println("Unsupported response", m)
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
			if err != tea.ErrProgramKilled {
				// tea library handles the error weirdly. It will return  an ErrProgramKilled when
				// the context has been canceled. This occurs when the program shutdowns, which should not
				// actually be an error
				fmt.Printf("Failed bubble tea program: %s\n", err)
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
