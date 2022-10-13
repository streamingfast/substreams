package tools

import (
	"context"
	"encoding/hex"
	"fmt"
	"github.com/streamingfast/substreams/store"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/descriptorpb"
	"strconv"
	"strings"

	"github.com/jhump/protoreflect/desc"
	"github.com/jhump/protoreflect/dynamic"
	"github.com/spf13/cobra"
	"github.com/streamingfast/dstore"
	"github.com/streamingfast/substreams/manifest"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/pipeline/execout/cachev1"
)

var decodeCmd = &cobra.Command{
	Use:          "decode",
	SilenceUsage: true,
}

var decodeModuleCmd = &cobra.Command{
	Use:          "output <manifest_file> <module_name> <output_url> <block_number>",
	Short:        "Decode output base 64 encoded bytes to protobuf data structure",
	RunE:         runDecodeModuleRunE,
	Args:         cobra.MinimumNArgs(4),
	SilenceUsage: true,
}

func init() {
	decodeModuleCmd.Flags().Uint64("save-interval", 1000, "Output save interval")
	//decodeStoresCmd.Flags().Uint64("save-interval", 1000, "Output save interval")

	decodeCmd.AddCommand(decodeModuleCmd)
	//decodeCmd.AddCommand(decodeStoresCmd)

	Cmd.AddCommand(decodeCmd)
}

func runDecodeModuleRunE(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	manifestPath := args[0]
	moduleName := args[1]
	storeUrl := args[2]
	saveInterval := mustGetUint64(cmd, "save-interval")
	blockNumber, err := strconv.ParseUint(args[3], 10, 64)
	if err != nil {
		return fmt.Errorf("converting blockNumber to uint: %w", err)
	}
	key := ""
	if len(args) > 4 {
		key = args[5]
	}

	zlog.Info("decoding module",
		zap.String("manifest_path", manifestPath),
		zap.String("module_name", moduleName),
		zap.String("store_url", storeUrl),
		zap.Uint64("block_number", blockNumber),
		zap.Uint64("save_internal", saveInterval),
		zap.String("key", key),
	)

	store, err := dstore.NewSimpleStore(storeUrl)
	if err != nil {
		return fmt.Errorf("initializing dstore for %q: %w", storeUrl, err)
	}

	pkg, err := manifest.NewReader(manifestPath).Read()
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

	hashes := manifest.NewModuleHashes()

	var matchingModule *pbsubstreams.Module
	for _, module := range pkg.Modules.Modules {
		if module.Name == moduleName {
			matchingModule = module
		}
	}
	if matchingModule == nil {
		return fmt.Errorf("module %q not found", moduleName)
	}

	moduleHash := hex.EncodeToString(hashes.HashModule(pkg.Modules, matchingModule, moduleGraph))
	zlog.Info("found module hash", zap.String("hash", moduleHash), zap.String("module", matchingModule.Name))

	startBlock := cachev1.ComputeStartBlock(blockNumber, saveInterval)

	switch matchingModule.Kind.(type) {
	case *pbsubstreams.Module_KindMap_:
		return searchMapModule(ctx, blockNumber, startBlock, saveInterval, moduleHash, matchingModule, store, protoFiles)
	case *pbsubstreams.Module_KindStore_:
		if key == "" {
			return fmt.Errorf("unable to search a store with a blank key")
		}
		return searchStoreModule(ctx, startBlock, saveInterval, moduleHash, key, matchingModule, store, protoFiles)
	}
	return fmt.Errorf("module has an unknown")
}

func searchMapModule(
	ctx context.Context,
	blockNumber,
	startBlock,
	saveInterval uint64,
	moduleHash string,
	module *pbsubstreams.Module,
	stateStore dstore.Store,
	protoFiles []*descriptorpb.FileDescriptorProto,
) error {
	moduleStore, err := stateStore.SubStore(moduleHash + "/outputs")
	if err != nil {
		return fmt.Errorf("can't find substore for hash %q: %w", moduleHash, err)
	}

	outputCache := cachev1.NewOutputCache(module.Name, moduleStore, saveInterval, zlog)
	zlog.Info("loading block from store", zap.Uint64("start_block", startBlock), zap.Uint64("block_num", blockNumber))
	found, err := outputCache.LoadAtBlock(ctx, startBlock)
	if err != nil {
		return fmt.Errorf("loading cache: %w", err)
	}
	if !found {
		return fmt.Errorf("can't find cache at block %d storeUrl %q", blockNumber, moduleStore.BaseURL().String())
	}

	fmt.Println()
	fmt.Printf("Found map output cache file containing block in bucket: %s\n", outputCache.String())
	outputBytes, found := outputCache.GetAtBlock(blockNumber)
	if !found {
		return fmt.Errorf("data not found at block %d", blockNumber)
	}

	if len(outputBytes) == 0 {
		fmt.Printf("Block %d found but payload is empty. Module did not produce data at block num.", blockNumber)
		return nil
	}

	return printObject(module, protoFiles, outputBytes)
}

func searchStoreModule(
	ctx context.Context,
	startBlock,
	saveInterval uint64,
	moduleHash string,
	key string,
	module *pbsubstreams.Module,
	stateStore dstore.Store,
	protoFiles []*descriptorpb.FileDescriptorProto,
) error {
	moduleStore, err := store.NewFullKV(module.Name, module.InitialBlock, moduleHash, module.GetKindStore().GetUpdatePolicy(), module.GetKindStore().GetValueType(), stateStore, zlog)
	if err != nil {
		return fmt.Errorf("initializing store for module %q: %w", module.Name, err)
	}

	if err = moduleStore.Load(ctx, (startBlock + saveInterval)); err != nil {
		return fmt.Errorf("unable to load file: %w", err)
	}

	bytes, found := moduleStore.GetLast(key)
	if !found {
		return fmt.Errorf("no data found for %q", key)
	}
	return printObject(module, protoFiles, bytes)
}

func printObject(module *pbsubstreams.Module, protoFiles []*descriptorpb.FileDescriptorProto, data []byte) error {
	protoDefinition := module.Output.GetType()
	fileDescriptors, err := desc.CreateFileDescriptors(protoFiles)
	if err != nil {
		return fmt.Errorf("unable to find file descriptors: %w", err)
	}

	var msgDesc *desc.MessageDescriptor
	for _, file := range fileDescriptors {
		msgDesc = file.FindMessage(strings.TrimPrefix(protoDefinition, "proto:"))
		if msgDesc != nil {
			dynMsg := dynamic.NewMessageFactoryWithDefaults().NewDynamicMessage(msgDesc)

			if err := dynMsg.Unmarshal(data); err != nil {
				return fmt.Errorf("unmarshalling outputBytes: %w", err)
			}
			cnt, err := dynMsg.MarshalJSON()
			if err != nil {
				return fmt.Errorf("marshalling json: %w", err)
			}
			fmt.Println(string(cnt))

			return nil
		}

		return fmt.Errorf("protobuf definition %s doesn't exist in the manifest", protoDefinition)
	}
	return nil
}
