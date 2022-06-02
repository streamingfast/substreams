package tui

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/dustin/go-humanize"
	"github.com/jhump/protoreflect/desc"
	"github.com/jhump/protoreflect/dynamic"
	"github.com/streamingfast/bstream"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"google.golang.org/protobuf/encoding/protojson"
)

type TUI struct {
	req               *pbsubstreams.Request
	pkg               *pbsubstreams.Package
	outputStreamNames []string

	outputMode string
	// Output flags
	decorateOutput    bool
	moduleWrapOutput  bool
	prettyPrintOutput bool

	prog *tea.Program

	decodeMsgTypes map[string]func(in []byte) string
	msgTypes       map[string]string
}

func New(req *pbsubstreams.Request, pkg *pbsubstreams.Package, outputStreamNames []string, outputMode string) *TUI {
	ui := &TUI{
		req:               req,
		pkg:               pkg,
		outputStreamNames: outputStreamNames,
		outputMode:        outputMode,
		decodeMsgTypes:    map[string]func(in []byte) string{},
		msgTypes:          map[string]string{},
	}

	ui.configureOutputMode()
	if ui.decorateOutput {
		ui.ensureTerminalLocked()
	}

	return ui
}

func (ui *TUI) Init() error {
	fileDescs, err := desc.CreateFileDescriptors(ui.pkg.ProtoFiles)
	if err != nil {
		return fmt.Errorf("couldn't convert, should do this check much earlier: %w", err)
	}

	toJson := ui.jsonFunc()
	decorate := ui.decorateOutput

	for _, mod := range ui.pkg.Modules.Modules {
		for _, outputStreamName := range ui.outputStreamNames {
			if mod.Name == outputStreamName {
				var msgType string
				var isStore bool
				switch modKind := mod.Kind.(type) {
				case *pbsubstreams.Module_KindStore_:
					isStore = true
					msgType = modKind.KindStore.ValueType
				case *pbsubstreams.Module_KindMap_:
					msgType = modKind.KindMap.OutputType
				}
				msgType = strings.TrimPrefix(msgType, "proto:")

				ui.msgTypes[mod.Name] = msgType

				var msgDesc *desc.MessageDescriptor
				for _, file := range fileDescs {
					msgDesc = file.FindMessage(msgType) //todo: make sure it works relatively-wise
					if msgDesc != nil {
						break
					}
				}
				modName := mod.Name
				decodeMsgType := func(in []byte) string {
					if msgDesc == nil {
						return "(unknown proto schema) " + decodeAsString(in)
					}
					msg := dynamic.NewMessageFactoryWithDefaults().NewDynamicMessage(msgDesc)
					if err := msg.Unmarshal(in); err != nil {
						fmt.Printf("error unmarshalling protobuf %s to map: %s\n", msgType, err.Error())
						//return decodeAsString(in)
						return ""
					}

					cnt, err := toJson(modName, msg)
					if err != nil {
						fmt.Printf("error encoding protobuf %s into json: %s\n", msgType, err)
						return decodeAsString(in)
					}

					return string(cnt)
				}

				if isStore {
					if msgDesc != nil {
						decodeMsgTypeWithIndent := func(in []byte) string {
							out := decodeMsgType(in)
							if decorate {
								out = strings.Replace(out, "\n", "\n    ", -1)
							}
							return out
						}
						ui.decodeMsgTypes[mod.Name] = decodeMsgTypeWithIndent
					} else {
						if msgType == "bytes" {
							ui.decodeMsgTypes[mod.Name] = decodeAsHex
						} else {
							// bigint, bigfloat, int64, float64, string
							ui.decodeMsgTypes[mod.Name] = decodeAsString
						}
					}

				} else {
					ui.decodeMsgTypes[mod.Name] = decodeMsgType
				}

			}
		}
	}
	return nil
}

func (ui *TUI) configureOutputMode() {
	switch ui.outputMode {
	case "ui":
		ui.prettyPrintOutput = true
		ui.decorateOutput = true
	case "jsonl":
	case "json":
		ui.prettyPrintOutput = true
	case "module-jsonl":
		ui.moduleWrapOutput = true
	case "module-json":
		ui.prettyPrintOutput = true
		ui.moduleWrapOutput = true
	}
}

func (ui *TUI) Cancel() {
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
		if ui.decorateOutput {
			ui.ensureTerminalUnlocked()
			return ui.decoratedBlockScopedData(m.Data)
		} else {
			return ui.jsonBlockScopedData(m.Data)
		}
	case *pbsubstreams.Response_Progress:
		if ui.decorateOutput {
			ui.ensureTerminalLocked()
			for _, module := range m.Progress.Modules {
				ui.prog.Send(module)
			}
		}
	case *pbsubstreams.Response_SnapshotData:
		fmt.Println("Incoming snapshot data")
	case *pbsubstreams.Response_SnapshotComplete:
		fmt.Println("Snapshot data dump complete")
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

func (ui *TUI) decoratedBlockScopedData(output *pbsubstreams.BlockScopedData) error {
	var s []string
	for _, out := range output.Outputs {
		for _, log := range out.Logs {
			s = append(s, fmt.Sprintf("%s: log: %s\n", out.Name, log))
		}

		switch data := out.Data.(type) {
		case *pbsubstreams.ModuleOutput_MapOutput:
			if len(data.MapOutput.Value) != 0 {
				decodeValue := ui.decodeMsgTypes[out.Name]
				msgType := ui.msgTypes[out.Name]
				if decodeValue != nil {
					cnt := decodeValue(data.MapOutput.GetValue())

					s = append(s, fmt.Sprintf("%s: message %q: %s\n", out.Name, msgType, cnt))
				} else {
					s = append(s, fmt.Sprintf("%s: message %q: ", out.Name, msgType))

					marshalledBytes, err := protojson.Marshal(data.MapOutput)
					if err != nil {
						return fmt.Errorf("return handler: marshalling: %w", err)
					}

					s = append(s, string(marshalledBytes))
				}
			}
		case *pbsubstreams.ModuleOutput_StoreDeltas:
			if len(data.StoreDeltas.Deltas) != 0 {
				s = append(s, fmt.Sprintf("%s: store deltas:\n", out.Name))
				decodeValue := ui.decodeMsgTypes[out.Name]
				for _, delta := range data.StoreDeltas.Deltas {
					s = append(s, fmt.Sprintf("  %s (%d) KEY: %q\n", delta.Operation.String(), delta.Ordinal, delta.Key))

					s = append(s, fmt.Sprintf("    OLD: %s\n", decodeValue(delta.OldValue)))
					s = append(s, fmt.Sprintf("    NEW: %s\n", decodeValue(delta.NewValue)))
				}
			}
		default:
			if data != nil {
				panic(fmt.Sprintf("unsupported module output data type %T", data))
			} else {
				//fmt.Println("received nil data for module", out.Name)
			}
		}
	}
	if len(s) != 0 {
		fmt.Println(strings.Join(s, ""))
	}
	return nil
}

func (ui *TUI) jsonBlockScopedData(output *pbsubstreams.BlockScopedData) error {
	encoder := protojson.MarshalOptions{}
	if ui.prettyPrintOutput {
		encoder.Multiline = true
		encoder.Indent = "  "
	}

	for _, out := range output.Outputs {
		switch data := out.Data.(type) {
		case *pbsubstreams.ModuleOutput_MapOutput:
			if len(data.MapOutput.Value) != 0 {
				decodeValue := ui.decodeMsgTypes[out.Name]
				if decodeValue != nil {
					cnt := decodeValue(data.MapOutput.GetValue())

					fmt.Println(cnt)
				} else {
					return fmt.Errorf("no function to decode type")
					// cnt, err := encoder.Marshal(data.MapOutput)a
					// if err != nil {
					// 	return fmt.Errorf("return handler: marshalling: %w", err)
					// }
					// fmt.Println(string(cnt))
				}
			}
		case *pbsubstreams.ModuleOutput_StoreDeltas:
			if len(data.StoreDeltas.Deltas) != 0 {
				cnt, err := encoder.Marshal(data.StoreDeltas)
				if err != nil {
					return fmt.Errorf("encoding deltas: %w", err)
				}
				if ui.moduleWrapOutput {
					cnt, err = json.MarshalIndent(moduleWrap{Module: out.Name, Data: json.RawMessage(cnt)}, "", "  ")
					if err != nil {
						return fmt.Errorf("encoding: module wrap: %w", err)
					}
				}
				fmt.Println(string(cnt))
			}
		default:
			if data != nil {
				panic(fmt.Sprintf("unsupported module output data type %T", data))
			} else {
				//fmt.Println("received nil data for module", out.Name)
			}
		}
	}
	return nil
}

func (ui *TUI) CleanUpTerminal() {
	if ui.prog != nil {
		if err := ui.prog.ReleaseTerminal(); err != nil {
			fmt.Println("failed releasing terminal:", err)
		}
	}
}

func failureProgressHandler(modName string, failure *pbsubstreams.ModuleProgress_Failed) error {
	fmt.Printf("---------------------- Module %s failed ---------------------\n", modName)
	for _, log := range failure.Logs {
		fmt.Printf("%s: %s\n", modName, log)
	}

	if failure.LogsTruncated {
		fmt.Println("<Logs Truncated>")
	}

	fmt.Printf("Error:\n%s", failure.Reason)
	return nil
}

func decodeAsString(in []byte) string { return fmt.Sprintf("%q", string(in)) }
func decodeAsHex(in []byte) string    { return "(hex) " + hex.EncodeToString(in) }

func printClock(block *pbsubstreams.BlockScopedData) {
	fmt.Printf("----------- BLOCK #%s (%d) ---------------\n", humanize.Comma(int64(block.Clock.Number)), block.Clock.Number)
}

func stepFromProto(step pbsubstreams.ForkStep) bstream.StepType {
	switch step {
	case pbsubstreams.ForkStep_STEP_NEW:
		return bstream.StepNew
	case pbsubstreams.ForkStep_STEP_UNDO:
		return bstream.StepUndo
	case pbsubstreams.ForkStep_STEP_IRREVERSIBLE:
		return bstream.StepIrreversible
	}
	return bstream.StepType(0)
}
