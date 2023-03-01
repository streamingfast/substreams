package tools

import (
	"context"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"

	"github.com/jhump/protoreflect/desc"
	"github.com/jhump/protoreflect/dynamic"
	"github.com/spf13/cobra"
	"github.com/streamingfast/cli"
	"github.com/streamingfast/dstore"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"

	"github.com/streamingfast/substreams/block"
	"github.com/streamingfast/substreams/manifest"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/storage/execout"
	"github.com/streamingfast/substreams/storage/store"
)

var decodeCmd = &cobra.Command{
	Use:          "decode",
	SilenceUsage: true,
}

var decodeOutputsModuleCmd = &cobra.Command{
	Use:   "outputs [<manifest_file>] <module_name> <output_url> <block_number> <key>",
	Short: "Decode outputs base 64 encoded bytes to protobuf data structure",
	Long: cli.Dedent(`
		When running this outputs command with a mapper or a store the key will be the block hash.  The manifest is optional as it will try to find a file named 
		'substreams.yaml' in current working directory if nothing entered. You may enter a directory that contains a 'substreams.yaml' 
		file in place of '<manifest_file>'.
	`),
	Example: string(cli.ExamplePrefixed("substreams tools decode outputs", `
		map_pools_created gs://[bucket-url-path] 12487090 pool:c772a65917d5da983b7fc3c9cfbfb53ef01aef7e
		uniswap-v3.spkg store_pools gs://[bucket-url-path] 12487090 pool:c772a65917d5da983b7fc3c9cfbfb53ef01aef7e
		dir-with-manifest store_pools gs://[bucket-url-path] 12487090 token:051cf5178f60e9def5d5a39b2a988a9f914107cb:dprice:eth
	`)),
	RunE:         runDecodeOutputsModuleRunE,
	Args:         cobra.RangeArgs(4, 5),
	SilenceUsage: true,
}

var decodeStatesModuleCmd = &cobra.Command{
	Use:   "states [<manifest_file>] <module_name> <output_url> <block_number> <key>",
	Short: "Decode states base 64 encoded bytes to protobuf data structure",
	Long: cli.Dedent(`
		Running the states command only works if the module is a store. If it is a map an error message will be returned
		to the user. The user needs to specify a key as it is required. The manifest is optional as it will try to find a file named 
		'substreams.yaml' in current working directory if nothing entered. You may enter a directory that contains a 'substreams.yaml' 
		file in place of '<manifest_file>'.
	`),
	Example: string(cli.ExamplePrefixed("substreams tools decode states", `
		store_eth_prices [bucket-url-path] 12487090 token:051cf5178f60e9def5d5a39b2a988a9f914107cb:dprice:eth
		dir-with-manifest store_pools [bucket-url-path] 12487090 pool:c772a65917d5da983b7fc3c9cfbfb53ef01aef7e
		uniswap-v3.spkg store_pools [bucket-url-path] 12487090 pool:c772a65917d5da983b7fc3c9cfbfb53ef01aef7e
	`)),
	RunE:         runDecodeStatesModuleRunE,
	Args:         cobra.RangeArgs(4, 5),
	SilenceUsage: true,
}

func init() {
	decodeOutputsModuleCmd.Flags().Uint64("save-interval", 1000, "Output save interval")
	decodeStatesModuleCmd.Flags().Uint64("save-interval", 1000, "states save interval")

	decodeCmd.AddCommand(decodeOutputsModuleCmd)
	decodeCmd.AddCommand(decodeStatesModuleCmd)

	Cmd.AddCommand(decodeCmd)
}

func runDecodeStatesModuleRunE(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	saveInterval := mustGetUint64(cmd, "save-interval")

	manifestPathRaw := ""
	if len(args) == 5 {
		manifestPathRaw = args[0]
		args = args[1:]
	}

	moduleName := args[0]
	storeURL := args[1]
	manifestPath, err := ResolveManifestFile(manifestPathRaw)
	if err != nil {
		return fmt.Errorf("resolving manifest: %w", err)
	}
	blockNumber, err := strconv.ParseUint(args[2], 10, 64)
	if err != nil {
		return fmt.Errorf("converting blockNumber to uint: %w", err)
	}
	key := args[3]

	zlog.Info("decoding module",
		zap.String("manifest_path", manifestPath),
		zap.String("module_name", moduleName),
		zap.String("store_url", storeURL),
		zap.Uint64("block_number", blockNumber),
		zap.Uint64("save_internal", saveInterval),
		zap.String("key", key),
	)

	objStore, err := dstore.NewStore(storeURL, "zst", "zstd", false)
	if err != nil {
		return fmt.Errorf("initializing dstore for %q: %w", storeURL, err)
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

	startBlock := execout.ComputeStartBlock(blockNumber, saveInterval)

	switch matchingModule.Kind.(type) {
	case *pbsubstreams.Module_KindMap_:
		return fmt.Errorf("no states are available for a mapper")
	case *pbsubstreams.Module_KindStore_:
		return searchStateModule(ctx, startBlock, saveInterval, moduleHash, key, matchingModule, objStore, protoFiles)
	}
	return fmt.Errorf("module has an unknown")
}

func runDecodeOutputsModuleRunE(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	saveInterval := mustGetUint64(cmd, "save-interval")

	manifestPathRaw := ""
	if len(args) == 5 {
		manifestPathRaw = args[0]
		args = args[1:]
	}

	moduleName := args[0]
	storeURL := args[1]
	manifestPath, err := ResolveManifestFile(manifestPathRaw)
	if err != nil {
		return fmt.Errorf("resolving manifest: %w", err)
	}
	blockNumber, err := strconv.ParseUint(args[2], 10, 64)
	if err != nil {
		return fmt.Errorf("converting blockNumber to uint: %w", err)
	}
	key := args[3]

	zlog.Info("decoding module",
		zap.String("manifest_path", manifestPath),
		zap.String("module_name", moduleName),
		zap.String("store_url", storeURL),
		zap.Uint64("block_number", blockNumber),
		zap.Uint64("save_internal", saveInterval),
		zap.String("key", key),
	)

	s, err := dstore.NewStore(storeURL, "zst", "zstd", false)
	if err != nil {
		return fmt.Errorf("initializing dstore for %q: %w", storeURL, err)
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

	startBlock := execout.ComputeStartBlock(blockNumber, saveInterval)

	switch matchingModule.Kind.(type) {
	case *pbsubstreams.Module_KindMap_:
		return searchOutputsModule(ctx, blockNumber, startBlock, saveInterval, moduleHash, matchingModule, s, protoFiles)
	case *pbsubstreams.Module_KindStore_:
		return searchOutputsModule(ctx, blockNumber, startBlock, saveInterval, moduleHash, matchingModule, s, protoFiles)
	}
	return fmt.Errorf("module has an unknown")
}

func searchOutputsModule(
	ctx context.Context,
	blockNumber,
	startBlock,
	saveInterval uint64,
	moduleHash string,
	module *pbsubstreams.Module,
	stateStore dstore.Store,
	protoFiles []*descriptorpb.FileDescriptorProto,
) error {
	modStore, err := execout.NewConfig(module.Name, module.InitialBlock, pbsubstreams.ModuleKindMap, moduleHash, stateStore, zlog)
	moduleStore, err := stateStore.SubStore(moduleHash + "/outputs")
	if err != nil {
		return fmt.Errorf("can't find substore for hash %q: %w", moduleHash, err)
	}

	targetRange := block.NewBoundedRange(module.InitialBlock, saveInterval, startBlock, startBlock-startBlock%saveInterval+saveInterval)
	outputCache := modStore.NewFile(targetRange)
	zlog.Info("loading block from store", zap.Uint64("start_block", startBlock), zap.Uint64("block_num", blockNumber))
	found, err := outputCache.Load(ctx)
	if err != nil {
		return fmt.Errorf("loading cache %s file %s : %w", moduleStore.BaseURL(), outputCache.String(), err)
	}
	if !found {
		return fmt.Errorf("can't find cache at block %d storeURL %q", blockNumber, moduleStore.BaseURL().String())
	}

	fmt.Println()
	payloadBytes, found := outputCache.GetAtBlock(blockNumber)
	if !found {
		return fmt.Errorf("data not found at block %d", blockNumber)
	}

	if len(payloadBytes) == 0 {
		fmt.Printf("RecordBlock %d found but payload is empty. Module did not produce data at block num.", blockNumber)
		return nil
	}

	return printObject(module, protoFiles, payloadBytes)
}

func searchStateModule(
	ctx context.Context,
	startBlock,
	saveInterval uint64,
	moduleHash string,
	key string,
	module *pbsubstreams.Module,
	stateStore dstore.Store,
	protoFiles []*descriptorpb.FileDescriptorProto,
) error {
	config, err := store.NewConfig(module.Name, module.InitialBlock, moduleHash, module.GetKindStore().GetUpdatePolicy(), module.GetKindStore().GetValueType(), stateStore)
	if err != nil {
		return fmt.Errorf("initializing store config module %q: %w", module.Name, err)
	}
	moduleStore := config.NewFullKV(zlog)
	if err = moduleStore.Load(ctx, startBlock+saveInterval); err != nil {
		return fmt.Errorf("unable to load file: %w", err)
	}

	bytes, found := moduleStore.GetLast(key)
	if !found {
		return fmt.Errorf("no data found for %q", key)
	}
	return printObject(module, protoFiles, bytes)
}

func printObject(module *pbsubstreams.Module, protoFiles []*descriptorpb.FileDescriptorProto, data []byte) error {
	protoDefinition := ""
	valuePrinted := false

	switch module.Kind.(type) {
	case *pbsubstreams.Module_KindMap_:
		protoDefinition = module.Output.GetType()
	case *pbsubstreams.Module_KindStore_:
		protoDefinition = module.Kind.(*pbsubstreams.Module_KindStore_).KindStore.ValueType
	default:
		return fmt.Errorf("invalid module kind: %q", module.Kind)
	}
	fileDescriptors, err := desc.CreateFileDescriptors(protoFiles)
	if err != nil {
		return fmt.Errorf("unable to find file descriptors: %w", err)
	}

	var msgDesc *desc.MessageDescriptor
	for _, file := range fileDescriptors {
		msgDesc = file.FindMessage(strings.TrimPrefix(protoDefinition, "proto:"))
		if msgDesc != nil {
			switch module.Kind.(type) {
			case *pbsubstreams.Module_KindMap_:
				dynMsg := dynamic.NewMessageFactoryWithDefaults().NewDynamicMessage(msgDesc)
				val, err := unmarshalData(data, dynMsg)
				if err != nil {
					return fmt.Errorf("unmarshalling data: %w", err)
				}
				fmt.Println(val)
				valuePrinted = true
			case *pbsubstreams.Module_KindStore_:
				deltas := &pbsubstreams.StoreDeltas{}
				_ = proto.Unmarshal(data, deltas)

				dynMsg := dynamic.NewMessageFactoryWithDefaults().NewDynamicMessage(msgDesc)

				value := ""
				for _, delta := range deltas.Deltas {
					value += fmt.Sprintf("> Key %s\n", delta.Key)
					value += fmt.Sprintln("----- New Value -----")
					val, err := unmarshalData(delta.NewValue, dynMsg)
					if err != nil {
						return fmt.Errorf("unmarshalling data: %w", err)
					}
					value += fmt.Sprintln(val)

					value += fmt.Sprintln("----- Old Value -----")
					val, err = unmarshalData(delta.OldValue, dynMsg)
					if err != nil {
						return fmt.Errorf("unmarshalling data: %w", err)
					}
					value += fmt.Sprintln(val)
				}

				fmt.Println(value)
				valuePrinted = true
			default:
				return fmt.Errorf("invalid module kind: %q", module.Kind)
			}
		}
	}

	if valuePrinted {
		return nil
	}

	fmt.Println(string(data))
	return nil
}

func unmarshalData(data []byte, dynMsg *dynamic.Message) (string, error) {
	if err := dynMsg.Unmarshal(data); err != nil {
		return "", fmt.Errorf("unmarshalling outputBytes: %w", err)
	}
	cnt, err := dynMsg.MarshalJSON()
	if err != nil {
		return "", fmt.Errorf("marshalling json: %w", err)
	}

	return string(cnt), nil
}
