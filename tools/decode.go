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
	Use:   "outputs [<manifest_file>] <module_name> <output_url> <block_number>",
	Short: "Decode outputs base 64 encoded bytes to protobuf data structure",
	Long: cli.Dedent(`
		When running this outputs command with a mapper or a store the key will be the block hash.  The manifest is optional as it will try to find a file named
		'substreams.yaml' in current working directory if nothing entered. You may enter a directory that contains a 'substreams.yaml'
		file in place of '<manifest_file>, or a link to a remote .spkg file, using urls gs://, http(s)://, ipfs://, etc.'.
	`),
	Example: string(cli.ExamplePrefixed("substreams tools decode outputs", `
		map_pools_created gs://[bucket-url-path] 12487090 pool:c772a65917d5da983b7fc3c9cfbfb53ef01aef7e
		uniswap-v3.spkg store_pools gs://[bucket-url-path] 12487090 pool:c772a65917d5da983b7fc3c9cfbfb53ef01aef7e
		dir-with-manifest store_pools gs://[bucket-url-path] 12487090 token:051cf5178f60e9def5d5a39b2a988a9f914107cb:dprice:eth
	`)),
	RunE:         runDecodeOutputsModuleRunE,
	Args:         cobra.RangeArgs(3, 4),
	SilenceUsage: true,
}

var decodeStatesModuleCmd = &cobra.Command{
	Use:   "states [<manifest_file>] <module_name> <output_url> <block_number> <key>",
	Short: "Decode states base 64 encoded bytes to protobuf data structure",
	Long: cli.Dedent(`
		Running the states command only works if the module is a store. If it is a map an error message will be returned
		to the user. The manifest is optional as it will try to find a file named
		'substreams.yaml' in current working directory if nothing entered. You may enter a directory that contains a 'substreams.yaml'
		file in place of '<manifest_file>, or a link to a remote .spkg file, using urls gs://, http(s)://, ipfs://, etc.'.
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

	manifestPath := ""
	if len(args) == 5 {
		manifestPath = args[0]
		args = args[1:]
	}

	moduleName := args[0]
	storeURL := args[1]
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

	manifestReader, err := manifest.NewReader(manifestPath, manifest.SkipPackageValidationReader())
	if err != nil {
		return fmt.Errorf("manifest reader: %w", err)
	}

	pkg, graph, err := manifestReader.Read()
	if err != nil {
		return fmt.Errorf("read manifest %q: %w", manifestPath, err)
	}

	protoFiles := pkg.ProtoFiles

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

	hash, err := hashes.HashModule(pkg.Modules, matchingModule, graph)
	if err != nil {
		panic(err)
	}
	moduleHash := hex.EncodeToString(hash)
	zlog.Info("found module hash", zap.String("hash", moduleHash), zap.String("module", matchingModule.Name))

	startBlock := execout.ComputeStartBlock(blockNumber, saveInterval)

	switch matchingModule.Kind.(type) {
	case *pbsubstreams.Module_KindMap_:
		return fmt.Errorf("no states are available for a mapper")
	case *pbsubstreams.Module_KindStore_:
		return searchStateModule(ctx, startBlock, moduleHash, key, matchingModule, objStore, protoFiles)
	}
	return fmt.Errorf("module has an unknown")
}

func runDecodeOutputsModuleRunE(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	saveInterval := mustGetUint64(cmd, "save-interval")

	manifestPath := ""
	if len(args) == 4 {
		manifestPath = args[0]
		args = args[1:]
	}

	moduleName := args[0]
	storeURL := args[1]

	requestedBlocks := block.ParseRange(args[2]) // FIXME: this panics on error :(

	zlog.Info("decoding module",
		zap.String("manifest_path", manifestPath),
		zap.String("module_name", moduleName),
		zap.String("store_url", storeURL),
		zap.Stringer("requested_block_range", requestedBlocks),
		zap.Uint64("save_internal", saveInterval),
	)

	s, err := dstore.NewStore(storeURL, "zst", "zstd", false)
	if err != nil {
		return fmt.Errorf("initializing dstore for %q: %w", storeURL, err)
	}

	manifestReader, err := manifest.NewReader(manifestPath, manifest.SkipPackageValidationReader())
	if err != nil {
		return fmt.Errorf("manifest reader: %w", err)
	}

	pkg, graph, err := manifestReader.Read()
	if err != nil {
		return fmt.Errorf("read manifest %q: %w", manifestPath, err)
	}

	protoFiles := pkg.ProtoFiles

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
	hash, err := hashes.HashModule(pkg.Modules, matchingModule, graph)
	if err != nil {
		return err
	}

	moduleHash := hex.EncodeToString(hash)
	zlog.Info("found module hash", zap.String("hash", moduleHash), zap.String("module", matchingModule.Name))

	startBlock := execout.ComputeStartBlock(requestedBlocks.StartBlock, saveInterval)
	if startBlock < matchingModule.InitialBlock {
		startBlock = matchingModule.InitialBlock
	}

	switch matchingModule.Kind.(type) {
	case *pbsubstreams.Module_KindMap_:
		return searchOutputsModule(ctx, requestedBlocks, startBlock, saveInterval, moduleHash, matchingModule, s, protoFiles)
	case *pbsubstreams.Module_KindStore_:
		return searchOutputsModule(ctx, requestedBlocks, startBlock, saveInterval, moduleHash, matchingModule, s, protoFiles)
	}
	return fmt.Errorf("module has an unknown")
}

func searchOutputsModule(
	ctx context.Context,
	requestedBlocks *block.Range,
	startBlock,
	saveInterval uint64,
	moduleHash string,
	module *pbsubstreams.Module,
	stateStore dstore.Store,
	protoFiles []*descriptorpb.FileDescriptorProto,
) error {
	modStore, err := execout.NewConfig(module.Name, module.InitialBlock, pbsubstreams.ModuleKindMap, moduleHash, stateStore, zlog)
	if err != nil {
		return fmt.Errorf("execout new config: %w", err)
	}

	moduleStore, err := stateStore.SubStore(moduleHash + "/outputs")
	if err != nil {
		return fmt.Errorf("can't find substore for hash %q: %w", moduleHash, err)
	}

	rng := block.NewRange(startBlock, startBlock-startBlock%saveInterval+saveInterval)

	outputCache := modStore.NewFile(rng)
	zlog.Info("loading block from store", zap.Uint64("start_block", startBlock), zap.Stringer("requested_block_range", requestedBlocks))
	if err := outputCache.Load(ctx); err != nil {
		if err == dstore.ErrNotFound {
			return fmt.Errorf("can't find cache at block %d storeURL %q", startBlock, moduleStore.BaseURL().String())
		}

		if err != nil {
			return fmt.Errorf("loading cache %s file %s : %w", moduleStore.BaseURL(), outputCache.String(), err)
		}
	}

	for i := requestedBlocks.StartBlock; i < requestedBlocks.ExclusiveEndBlock; i++ {
		payloadBytes, found := outputCache.GetAtBlock(i)
		if !found {
			continue
		}

		fmt.Println("Block", i)
		if len(payloadBytes) == 0 {
			continue
		}
		if err := printObject(module, protoFiles, payloadBytes); err != nil {
			return fmt.Errorf("printing object: %w", err)
		}
	}
	return nil
}

func searchStateModule(
	ctx context.Context,
	startBlock uint64,
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

	file := store.NewCompleteFileInfo(module.Name, module.InitialBlock, startBlock)
	if err = moduleStore.Load(ctx, file); err != nil {
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
			case *pbsubstreams.Module_KindMap_, *pbsubstreams.Module_KindStore_:
				dynMsg := dynamic.NewMessageFactoryWithDefaults().NewDynamicMessage(msgDesc)
				val, err := unmarshalData(data, dynMsg)
				if err != nil {
					return fmt.Errorf("unmarshalling data: %w", err)
				}
				fmt.Println(val)
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
