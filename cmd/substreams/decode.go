package main

import (
	"encoding/hex"
	"fmt"
	"github.com/jhump/protoreflect/desc"
	"github.com/jhump/protoreflect/dynamic"
	"github.com/spf13/cobra"
	"github.com/streamingfast/substreams/manifest"
	"strings"
)

// todo: split this on 2 commands: for store and mapper output
var decodeCmd = &cobra.Command{
	Use:          "decode <manifest_file> <protobuf_definition> <output_url> <block_number>",
	Short:        "Decode base 64 encoded bytes to protobuf data structure",
	RunE:         runDecode,
	Args:         cobra.ExactArgs(3),
	SilenceUsage: true,
}

//todo:
// add block number, gcp link and decode base64 field is payload

func init() {
	decodeCmd.Flags().Uint("save-interval", 1000)
	rootCmd.AddCommand(decodeCmd)
}

func runDecode(cmd *cobra.Command, args []string) error {
	manifestPath := args[0]
	manifestReader := manifest.NewReader(manifestPath)
	protobufDefinition := args[1]
	protobufHex := args[2]

	pkg, err := manifestReader.Read()
	if err != nil {
		return fmt.Errorf("read manifest %q: %w", manifestPath, err)
	}

	protoFiles := pkg.ProtoFiles
	if len(protoFiles) == 0 {
		return fmt.Errorf("no protobuf file definitions in the manifest")
	}

	fileDescriptors, err := desc.CreateFileDescriptors(protoFiles)
	var msgDesc *desc.MessageDescriptor
	for _, file := range fileDescriptors {
		msgDesc = file.FindMessage(strings.TrimPrefix(protobufDefinition, "proto:"))
		if msgDesc != nil {
			dynMsg := dynamic.NewMessageFactoryWithDefaults().NewDynamicMessage(msgDesc)

			b, err := hex.DecodeString(protobufHex)
			if err != nil {
				return fmt.Errorf("decoding hex: %w", err)
			}
			if err := dynMsg.Unmarshal(b); err != nil {

			}
			cnt, err := dynMsg.MarshalJSON()
			fmt.Println(string(cnt))

			return nil
		}
	}

	return fmt.Errorf("protobuf definition %s doesn't exist in the manifest", protobufDefinition)
}
