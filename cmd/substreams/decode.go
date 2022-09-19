package main

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/jhump/protoreflect/desc"
	"github.com/jhump/protoreflect/dynamic"
	"github.com/spf13/cobra"
	"github.com/streamingfast/dstore"
	"github.com/streamingfast/substreams/block"
	"github.com/streamingfast/substreams/manifest"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/pipeline/outputs"
	"github.com/streamingfast/substreams/state"
)

var decodeOutputsCmd = &cobra.Command{
	Use:          "output <manifest_file> <module_name> <output_url> <block_number>",
	Short:        "Decode output base 64 encoded bytes to protobuf data structure",
	RunE:         runDecodeOutput,
	Args:         cobra.ExactArgs(4),
	SilenceUsage: true,
}

var decodeStoresCmd = &cobra.Command{
	Use:          "store <manifest_file> <module_name> <stores_url> <block_number> <key>",
	Short:        "Decode store base 64 encoded bytes to protobuf data structure",
	RunE:         runDecodeStore,
	Args:         cobra.ExactArgs(5),
	SilenceUsage: true,
}

var decodeCmd = &cobra.Command{
	Use:          "decode",
	SilenceUsage: true,
}

func init() {
	decodeOutputsCmd.Flags().Uint64("save-interval", 1000, "Output save interval")
	decodeStoresCmd.Flags().Uint64("save-interval", 1000, "Output save interval")

	decodeCmd.AddCommand(decodeOutputsCmd)
	decodeCmd.AddCommand(decodeStoresCmd)

	rootCmd.AddCommand(decodeCmd)
}

func runDecodeOutput(cmd *cobra.Command, args []string) error {
	manifestPath := args[0]
	manifestReader := manifest.NewReader(manifestPath)
	moduleName := args[1]
	storeUrl := args[2]
	blockNumber, err := strconv.ParseUint(args[3], 10, 64)
	if err != nil {
		return fmt.Errorf("converting blockNumber to uint: %w", err)
	}

	saveInterval := mustGetUint64(cmd, "save-interval")
	startBlock := outputs.ComputeStartBlock(blockNumber, saveInterval)

	store, _, err := dstore.NewStoreFromURL(storeUrl)
	if err != nil {
		return fmt.Errorf("initializing dstore for %q: %w", storeUrl, err)
	}

	pkg, err := manifestReader.Read()
	if err != nil {
		return fmt.Errorf("read manifest %q: %w", manifestPath, err)
	}

	protoFiles := pkg.ProtoFiles
	if len(protoFiles) == 0 {
		return fmt.Errorf("no protobuf file definitions in the manifest")
	}

	moduleGraph, err := manifest.NewModuleGraph(pkg.Modules.Modules)
	if err != nil {
		return fmt.Errorf("processing module graph %w", err)
	}

	var hash string
	var protoDefinition string
	for _, module := range pkg.Modules.Modules {
		if module.Name == moduleName {
			hash = manifest.HashModuleAsString(pkg.Modules, moduleGraph, module)
			protoDefinition = module.Output.GetType()
		}
	}

	if hash == "" {
		return fmt.Errorf("module name not found %q", moduleName)
	}

	moduleStore, err := store.SubStore(hash + "/outputs")
	if err != nil {
		return fmt.Errorf("can't find substore for hash %q: %w", hash, err)
	}

	outputCache := outputs.NewOutputCache(moduleName, moduleStore, saveInterval, zlog)
	found, err := outputCache.LoadAtBlock(cmd.Context(), startBlock)
	if err != nil {
		return fmt.Errorf("loading cache: %w", err)
	}

	if !found {
		return fmt.Errorf("can't find cache at block %q storeUrl %q", startBlock, moduleStore.BaseURL().String())
	}

	outputBytes, found := outputCache.GetAtBlock(blockNumber)
	if !found {
		return fmt.Errorf("data not found at block %q", blockNumber)
	}

	fileDescriptors, err := desc.CreateFileDescriptors(protoFiles)
	var msgDesc *desc.MessageDescriptor
	for _, file := range fileDescriptors {
		msgDesc = file.FindMessage(strings.TrimPrefix(protoDefinition, "proto:"))
		if msgDesc != nil {
			dynMsg := dynamic.NewMessageFactoryWithDefaults().NewDynamicMessage(msgDesc)

			if err := dynMsg.Unmarshal(outputBytes); err != nil {
				return fmt.Errorf("unmarshalling outputBytes: %w", err)
			}
			cnt, err := dynMsg.MarshalJSON()
			if err != nil {
				return fmt.Errorf("marshalling json: %w", err)
			}
			fmt.Println(string(cnt))

			return nil
		}
	}

	return fmt.Errorf("protobuf definition %s doesn't exist in the manifest", protoDefinition)
}

func runDecodeStore(cmd *cobra.Command, args []string) error {
	manifestPath := args[0]
	manifestReader := manifest.NewReader(manifestPath)
	moduleName := args[1]
	storeUrl := args[2]
	blockNumber, err := strconv.ParseUint(args[3], 10, 64)
	if err != nil {
		return fmt.Errorf("converting blockNumber to uint: %w", err)
	}
	key := args[4]

	saveInterval := mustGetUint64(cmd, "save-interval")
	startBlock := outputs.ComputeStartBlock(blockNumber, saveInterval)

	store, _, err := dstore.NewStoreFromURL(storeUrl)
	if err != nil {
		return fmt.Errorf("initializing dstore for %q: %w", storeUrl, err)
	}

	pkg, err := manifestReader.Read()
	if err != nil {
		return fmt.Errorf("read manifest %q: %w", manifestPath, err)
	}

	protoFiles := pkg.ProtoFiles
	if len(protoFiles) == 0 {
		return fmt.Errorf("no protobuf file definitions in the manifest")
	}

	moduleGraph, err := manifest.NewModuleGraph(pkg.Modules.Modules)
	if err != nil {
		return fmt.Errorf("processing module graph %w", err)
	}

	var hash string
	var protoDefinition string
	var pbModule *pbsubstreams.Module
	for _, module := range pkg.Modules.Modules {
		if module.Name == moduleName {
			hash = manifest.HashModuleAsString(pkg.Modules, moduleGraph, module)
			protoDefinition = module.GetKindStore().GetValueType()
			pbModule = module
		}
	}

	if hash == "" {
		return fmt.Errorf("module name not found %q", moduleName)
	}

	moduleStore, err := state.NewStore(moduleName, saveInterval, pbModule.InitialBlock, hash, pbModule.GetKindStore().GetUpdatePolicy(), pbModule.GetKindStore().GetValueType(), store, zlog)
	if err != nil {
		return fmt.Errorf("initializing store for module %q: %w", moduleName, err)
	}

	moduleStore, err = moduleStore.LoadFrom(cmd.Context(), &block.Range{
		StartBlock:        pbModule.InitialBlock,
		ExclusiveEndBlock: startBlock + saveInterval,
	})

	bytes, found := moduleStore.GetLast(key)
	if !found {
		return fmt.Errorf("no data found for %q", key)
	}

	fileDescriptors, err := desc.CreateFileDescriptors(protoFiles)
	var msgDesc *desc.MessageDescriptor
	for _, file := range fileDescriptors {
		msgDesc = file.FindMessage(strings.TrimPrefix(protoDefinition, "proto:"))
		if msgDesc != nil {
			dynMsg := dynamic.NewMessageFactoryWithDefaults().NewDynamicMessage(msgDesc)

			if err := dynMsg.Unmarshal(bytes); err != nil {
				return fmt.Errorf("unmarshalling outputBytes: %w", err)
			}
			cnt, err := dynMsg.MarshalJSON()
			if err != nil {
				return fmt.Errorf("marshalling json: %w", err)
			}
			fmt.Println(string(cnt))

			return nil
		}
	}

	return fmt.Errorf("protobuf definition %s doesn't exist in the manifest", protoDefinition)
}
