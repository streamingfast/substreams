package decode

import (
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/jhump/protoreflect/desc"
	"github.com/jhump/protoreflect/dynamic"
	"github.com/streamingfast/substreams/manifest"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/anypb"
)

func NewPrintReturnHandler(manif *manifest.Manifest, outputStreamName string) func(any *anypb.Any) error {
	var msgType string
	var isStore bool
	for _, mod := range manif.Modules {
		if mod.Name == outputStreamName {
			if mod.Kind == "store" {
				isStore = true
				msgType = mod.ValueType
			} else {
				msgType = mod.Output.Type
			}
		}
	}

	msgType = strings.TrimPrefix(msgType, "proto:")

	var msgDesc *desc.MessageDescriptor
	for _, file := range manif.ProtoDescs {
		msgDesc = file.FindMessage(msgType) //todo: make sure it works relatively-wise
		if msgDesc != nil {
			break
		}
	}

	defaultHandler := func(any *anypb.Any) error {
		if any == nil {
			return nil
		}

		fmt.Printf("Message %q:\n", msgType)
		fmt.Println(protojson.Marshal(any))
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
			if msgType == "string" || msgType == "float" || msgType == "int" {
				decodeValue = decodeAsString
			} else {
				decodeValue = decodeAsHex
			}
		}

		return func(any *anypb.Any) error {
			if any == nil {
				return nil
			}
			d := &pbsubstreams.StoreDeltas{}
			if err := any.UnmarshalTo(d); err != nil {
				fmt.Printf("Error decoding store deltas: %s\n", err)
				fmt.Printf("Raw StoreDeltas bytes: %s\n", decodeAsHex(any.Value))
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
			return func(any *anypb.Any) error {
				if any == nil {
					return nil
				}

				cnt := decodeMsgType(any.GetValue())

				fmt.Printf("Message %q: %s\n", msgType, string(cnt))

				return nil
			}
		} else {
			return defaultHandler
		}
	}
}
