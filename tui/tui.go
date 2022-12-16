package tui

import (
	"fmt"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jhump/protoreflect/desc"
	"github.com/mattn/go-isatty"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

type TUI struct {
	req               *pbsubstreams.Request
	pkg               *pbsubstreams.Package
	outputStreamNames []string

	// Output mode flags
	isTerminal        bool
	decorateOutput    bool
	prettyPrintOutput bool

	prog          *tea.Program
	seenFirstData bool

	msgDescs       map[string]*desc.MessageDescriptor
	decodeMsgTypes map[string]func(in []byte) string
	msgTypes       map[string]string
}

func New(req *pbsubstreams.Request, pkg *pbsubstreams.Package, outputStreamNames []string) *TUI {
	ui := &TUI{
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

	if ui.decorateOutput {
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
			outputMode = "ui"
		} else {
			outputMode = "json"
		}
	}

	switch outputMode {
	case "ui":
		ui.prettyPrintOutput = true
		ui.decorateOutput = true
	case "jsonl":
	case "json":
		ui.prettyPrintOutput = true
	default:
		return fmt.Errorf("output mode %q invalid, choose from: ui, json, jsonl", outputMode)
	}
	return nil
}

func (ui *TUI) Cancel() {
	if ui.prog == nil {
		return
	}
	err := ui.prog.ReleaseTerminal()
	if err != nil {
		_ = fmt.Errorf("releasing terminal: %w", err)
	}
	// cancel a context or something we got from upstream, passing the command-line control here.
	// a Shutter or something
}

func (ui *TUI) IncomingMessage(resp *pbsubstreams.Response) error {
	switch m := resp.Message.(type) {
	case *pbsubstreams.Response_Data:
		if ui.decorateOutput {
			printClock(m.Data)
		}
		if m.Data == nil {
			return nil
		}
		if len(m.Data.Outputs) == 0 {
			return nil
		}
		ui.seenFirstData = true
		if ui.decorateOutput {
			ui.ensureTerminalUnlocked()
			return ui.decoratedBlockScopedData(m.Data)
		} else {
			return ui.jsonBlockScopedData(m.Data)
		}
	case *pbsubstreams.Response_Progress:
		if ui.seenFirstData {
			ui.formatPostDataProgress(m)
		} else {
			if ui.decorateOutput {
				ui.ensureTerminalLocked()
				for _, module := range m.Progress.Modules {
					ui.prog.Send(module)
				}
			}
		}
	case *pbsubstreams.Response_DebugSnapshotData:
		if ui.decorateOutput {
			ui.ensureTerminalUnlocked()
			return ui.decoratedSnapshotData(m.DebugSnapshotData)
		} else {
			return ui.jsonSnapshotData(m.DebugSnapshotData)
		}

	case *pbsubstreams.Response_DebugSnapshotComplete:
		if ui.decorateOutput {
			fmt.Println("Snapshot data dump complete")
		}
	default:
		fmt.Println("Unsupported response")
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
		if err := ui.prog.Start(); err != nil {
			fmt.Printf("Failed bubble tea program: %s\n", err)
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
