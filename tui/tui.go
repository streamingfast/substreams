package tui

import (
	"encoding/hex"
	"fmt"
	"strings"

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
	prettyPrint       bool

	prog *tea.Program

	decodeMsgTypes map[string]func(in []byte) string
	msgTypes       map[string]string
}

func New(req *pbsubstreams.Request, pkg *pbsubstreams.Package, outputStreamNames []string, prettyPrint bool) *TUI {
	ui := &TUI{
		req:               req,
		pkg:               pkg,
		outputStreamNames: outputStreamNames,
		prettyPrint:       prettyPrint,
		decodeMsgTypes:    map[string]func(in []byte) string{},
		msgTypes:          map[string]string{},
	}
	ui.prog = tea.NewProgram(newModel(ui))
	return ui
}

func (ui *TUI) Init() error {
	fileDescs, err := desc.CreateFileDescriptors(ui.pkg.ProtoFiles)
	if err != nil {
		return fmt.Errorf("couldn't convert, should do this check much earlier: %w", err)
	}

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

					var cnt []byte
					var err error
					if ui.prettyPrint {
						cnt, err = msg.MarshalJSONIndent()
					} else {
						cnt, err = msg.MarshalJSON()
					}

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
							return strings.Replace(out, "\n", "\n    ", -1)
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

func (ui *TUI) Start() {
	if err := ui.prog.Start(); err != nil {
		fmt.Printf("Failed bubble tea program: %s\n", err)
	}
}

func (ui *TUI) IncomingMessage(resp *pbsubstreams.Response) error {
	switch m := resp.Message.(type) {
	case *pbsubstreams.Response_Data:
		return ui.blockScopedData(m.Data)
	case *pbsubstreams.Response_Progress:
		for _, module := range m.Progress.Modules {
			ui.prog.Send(module)
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

func (ui *TUI) blockScopedData(output *pbsubstreams.BlockScopedData) error {
	ui.prog.Send(output.Clock)
	//printClock(output)
	if output == nil {
		return nil
	}
	if len(output.Outputs) == 0 {
		return nil
	}

	for _, out := range output.Outputs {
		for _, log := range out.Logs {
			fmt.Printf("%s: log: %s\n", out.Name, log)
		}

		switch data := out.Data.(type) {
		case *pbsubstreams.ModuleOutput_MapOutput:
			if len(data.MapOutput.Value) != 0 {
				decodeValue := ui.decodeMsgTypes[out.Name]
				msgType := ui.msgTypes[out.Name]
				if decodeValue != nil {
					cnt := decodeValue(data.MapOutput.GetValue())

					fmt.Printf("%s: message %q: %s\n", out.Name, msgType, cnt)
				} else {
					fmt.Printf("%s: message %q: ", out.Name, msgType)

					marshalledBytes, err := protojson.Marshal(data.MapOutput)
					if err != nil {
						return fmt.Errorf("return handler: marshalling: %w", err)
					}

					fmt.Println(marshalledBytes)
				}
			}

		case *pbsubstreams.ModuleOutput_StoreDeltas:
			if len(data.StoreDeltas.Deltas) != 0 {
				fmt.Printf("%s: store deltas:\n", out.Name)
				decodeValue := ui.decodeMsgTypes[out.Name]
				for _, delta := range data.StoreDeltas.Deltas {
					fmt.Printf("  %s (%d) KEY: %q\n", delta.Operation.String(), delta.Ordinal, delta.Key)

					fmt.Printf("    OLD: %s\n", decodeValue(delta.OldValue))
					fmt.Printf("    NEW: %s\n", decodeValue(delta.NewValue))
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
	return nil
}

func (ui *TUI) CleanUpTerminal() {
	if err := ui.prog.ReleaseTerminal(); err != nil {
		fmt.Println("failed releasing terminal:", err)
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
	fmt.Printf("\n----------- %s BLOCK #%s (%d) ---------------\n",
		strings.ToUpper(stepFromProto(block.Step).String()),
		humanize.Comma(int64(block.Clock.Number)), block.Clock.Number)
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
