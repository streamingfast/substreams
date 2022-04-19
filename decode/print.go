package decode

import (
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/jhump/protoreflect/desc"
	"github.com/jhump/protoreflect/dynamic"
	"github.com/streamingfast/bstream"
	"github.com/streamingfast/substreams"
	"github.com/streamingfast/substreams/manifest"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"google.golang.org/protobuf/encoding/protojson"
)

func NewPrintReturnHandler(manif *manifest.Manifest, outputStreamNames []string) substreams.ReturnFunc {

	var isStore bool
	for _, mod := range manif.Modules {
		for _, outputStreamName := range outputStreamNames {
			if mod.Name == outputStreamName {
				var msgType string
				if mod.Kind == "store" {
					isStore = true
					msgType = mod.ValueType
				} else {
					msgType = mod.Output.Type
				}
				msgType = strings.TrimPrefix(msgType, "proto:")

				var msgDesc *desc.MessageDescriptor
				for _, file := range manif.ProtoDescs {
					msgDesc = file.FindMessage(msgType) //todo: make sure it works relatively-wise
					if msgDesc != nil {
						break
					}
				}

			}
		}
	}

	defaultHandler := func(output *pbsubstreams.BlockScopedData) error {
		printBlock(output)
		if output == nil {
			return nil
		}

		fmt.Printf("Message %q:\n", msgType)

		marshalledBytes, err := protojson.Marshal(output.GetValue())
		if err != nil {
			return fmt.Errorf("return handler: marshalling: %w", err)
		}

		fmt.Println(marshalledBytes)
		return nil
	}

	decodeAsString := func(in []byte) string { return fmt.Sprintf("%q", string(in)) }
	decodeAsHex := func(in []byte) string { return "(hex) " + hex.EncodeToString(in) }
	decodeMsgType := func(in []byte) string {
		msg := dynamic.NewMessageFactoryWithDefaults().NewDynamicMessage(msgDesc)
		if err := msg.Unmarshal(in); err != nil {
			fmt.Printf("error unmarshalling protobuf %s to map: %s\n", msgType, err)
			return decodeAsString(in)
		}

		cnt, err := msg.MarshalJSONIndent()
		if err != nil {
			fmt.Printf("error encoding protobuf %s into json: %s\n", msgType, err)
			return decodeAsString(in)
		}

		return string(cnt)
	}
	decodeMsgTypeWithIndent := func(in []byte) string {
		out := decodeMsgType(in)
		return strings.Replace(out, "\n", "\n    ", -1)
	}

	if isStore {
		var decodeValue func(in []byte) string
		if msgDesc != nil {
			decodeValue = decodeMsgTypeWithIndent
		} else {
			if msgType == "bytes" {
				decodeValue = decodeAsHex
			} else {
				// bigint, bigfloat, int64, float64, string
				decodeValue = decodeAsString
			}
		}

		return func(output *pbsubstreams.BlockScopedData) error {
			printBlock(output)
			if output == nil {
				return nil
			}
			d := &pbsubstreams.StoreDeltas{}
			if err := output.Value.UnmarshalTo(d); err != nil {
				fmt.Printf("Error decoding store deltas: %s\n", err)
				fmt.Printf("Raw StoreDeltas bytes: %s\n", decodeAsHex(output.Value.Value))
			}

			fmt.Printf("Store deltas for %q:\n", outputStreamName)
			for _, delta := range d.Deltas {
				fmt.Printf("  %s (%d) KEY: %q\n", delta.Operation.String(), delta.Ordinal, delta.Key)

				fmt.Printf("    OLD: %s\n", decodeValue(delta.OldValue))
				fmt.Printf("    NEW: %s\n", decodeValue(delta.NewValue))
			}
			return nil
		}
	} else {
		if msgDesc != nil {
			return func(output *pbsubstreams.BlockScopedData) error {
				printBlock(output)
				if output == nil {
					return nil
				}

				cnt := decodeMsgType(output.Value.GetValue())

				fmt.Printf("Message %q: %s\n", msgType, cnt)

				return nil
			}
		} else {
			return defaultHandler
		}
	}
}

func printBlock(block *pbsubstreams.BlockScopedData) {
	fmt.Printf("----------- BLOCK: %d (%s) ---------------\n", block.Clock.Number, stepFromProto(block.Step))
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
