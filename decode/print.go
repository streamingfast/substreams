package decode

import (
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/dustin/go-humanize"
	"github.com/jhump/protoreflect/desc"
	"github.com/jhump/protoreflect/dynamic"
	"github.com/streamingfast/bstream"
	"github.com/streamingfast/substreams"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"google.golang.org/protobuf/encoding/protojson"
)

func NewPrintReturnHandler(pkg *pbsubstreams.Package, outputStreamNames []string, prettyPrint bool) substreams.ResponseFunc {
	decodeMsgTypes := map[string]func(in []byte) string{}
	msgTypes := map[string]string{}

	fileDescs, err := desc.CreateFileDescriptors(pkg.ProtoFiles)
	if err != nil {
		panic("couldn't convert, should do this check much earlier: " + err.Error())
	}

	for _, mod := range pkg.Modules.Modules {
		for _, outputStreamName := range outputStreamNames {
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

				msgTypes[mod.Name] = msgType

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
					if prettyPrint {
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
						decodeMsgTypes[mod.Name] = decodeMsgTypeWithIndent
					} else {
						if msgType == "bytes" {
							decodeMsgTypes[mod.Name] = decodeAsHex
						} else {
							// bigint, bigfloat, int64, float64, string
							decodeMsgTypes[mod.Name] = decodeAsString
						}
					}

				} else {
					decodeMsgTypes[mod.Name] = decodeMsgType
				}

			}
		}
	}

	blockScopedData := func(output *pbsubstreams.BlockScopedData) error {
		printClock(output)
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
					decodeValue := decodeMsgTypes[out.Name]
					msgType := msgTypes[out.Name]
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
					decodeValue := decodeMsgTypes[out.Name]
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
					fmt.Println("received nil data for module", out.Name)
				}
			}
		}
		return nil
	}

	progress := func(progress *pbsubstreams.ModulesProgress) error {
		if failedModule := firstFailedModuleProgress(progress); failedModule != nil {
			if err := failureProgressHandler(progress); err != nil {
				fmt.Printf("FAILURE PROGRESS HANDLER ERROR: %s\n", err)
			}
		}
		for _, moduleProgress := range progress.Modules {
			fmt.Printf("module:%s %s\n", moduleProgress.Name, moduleProgress.ProcessedRanges)

		}
		return nil
	}

	return func(resp *pbsubstreams.Response) error {
		switch m := resp.Message.(type) {
		case *pbsubstreams.Response_Data:
			return blockScopedData(m.Data)
		case *pbsubstreams.Response_Progress:
			return progress(m.Progress)
		case *pbsubstreams.Response_SnapshotData:
			fmt.Println("Incoming snapshot data")
		case *pbsubstreams.Response_SnapshotComplete:
			fmt.Println("Snapshot data dump complete")
		default:
			fmt.Println("Unsupported response")
		}
		return nil
	}
}

func failureProgressHandler(progress *pbsubstreams.ModulesProgress) error {
	failedModule := firstFailedModuleProgress(progress)
	if failedModule == nil {
		return nil
	}

	fmt.Printf("---------------------- Module %s failed ---------------------\n", failedModule.Name)
	for _, module := range progress.Modules {
		for _, log := range module.FailureLogs {
			fmt.Printf("%s: %s\n", module.Name, log)
		}

		if module.FailureLogsTruncated {
			fmt.Println("<Logs Truncated>")
		}
	}

	fmt.Printf("Error:\n%s", failedModule.FailureReason)
	return nil
}

func firstFailedModuleProgress(modulesProgress *pbsubstreams.ModulesProgress) *pbsubstreams.ModuleProgress {
	for _, module := range modulesProgress.Modules {
		if module.Failed == true {
			return module
		}
	}

	return nil
}

func decodeAsString(in []byte) string { return fmt.Sprintf("%q", string(in)) }
func decodeAsHex(in []byte) string    { return "(hex) " + hex.EncodeToString(in) }

func printClock(block *pbsubstreams.BlockScopedData) {
	fmt.Printf("----------- %s BLOCK #%s (%d) ---------------\n",
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
