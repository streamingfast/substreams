package tools

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/streamingfast/substreams/block"

	"github.com/streamingfast/cli"

	"google.golang.org/protobuf/proto"

	"github.com/jhump/protoreflect/dynamic"

	"github.com/streamingfast/substreams/storage/execout"
	"github.com/streamingfast/substreams/storage/store"

	"github.com/jhump/protoreflect/desc"
	"github.com/spf13/cobra"
	"github.com/streamingfast/dstore"
	"github.com/streamingfast/substreams/manifest"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/descriptorpb"
)

var decodeCmd = &cobra.Command{
	Use:          "decode",
	SilenceUsage: true,
}

var decodeOutputsModuleCmd = &cobra.Command{
	Use:   "outputs <module_name> <output_url> <block_number> <key> <manifest_file>",
	Short: "Decode outputs base 64 encoded bytes to protobuf data structure",
	Long: cli.Dedent(`
		When running this outputs command with a mapper or a store the key will be the block hash. The key is optional
		as it will return all the keys on the given block. The manifest is also optional as it will try to find one in 
		your pwd.
	`),
	Example: cli.Dedent(`
		substreams tools decode outputs map_pools_created [bucket-url-path] 12487090 <optional_key>
		substreams tools decode outputs store_pools [bucket-url-path] 12487090 <optional_key> <optional_manifest_file>
	`),
	RunE:         runDecodeOutputsModuleRunE,
	Args:         cobra.MinimumNArgs(3),
	SilenceUsage: true,
}

var decodeStatesModuleCmd = &cobra.Command{
	Use:   "states <module_name> <output_url> <block_number> <key> <manifest_file>",
	Short: "Decode states base 64 encoded bytes to protobuf data structure",
	Long: cli.Dedent(`
		Running the states command only works if the module is a store. If it is a map an error message will be returned
		to the user. The user needs to specify a key as it is required. The manifest is optional as it will try to find 
		one in your pwd.
	`),
	Example: cli.Dedent(`
		substreams tools decode states store_eth_prices [bucket-url-path] 12487090 token:051cf5178f60e9def5d5a39b2a988a9f914107cb:dprice:eth
		substreams tools decode states store_pools [bucket-url-path] 12487090 pool:c772a65917d5da983b7fc3c9cfbfb53ef01aef7e [optional-manifest-file]
	`),
	RunE:         runDecodeStatesModuleRunE,
	Args:         cobra.MinimumNArgs(4),
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
		key = args[4]
	}

	zlog.Info("decoding module",
		zap.String("manifest_path", manifestPath),
		zap.String("module_name", moduleName),
		zap.String("store_url", storeUrl),
		zap.Uint64("block_number", blockNumber),
		zap.Uint64("save_internal", saveInterval),
		zap.String("key", key),
	)

	objStore, err := dstore.NewStore(storeUrl, "zst", "zstd", false)
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
	moduleName := args[0]
	storeUrl := args[1]
	saveInterval := mustGetUint64(cmd, "save-interval")
	blockNumber, err := strconv.ParseUint(args[2], 10, 64)
	if err != nil {
		return fmt.Errorf("converting blockNumber to uint: %w", err)
	}

	key := ""
	if len(args) > 3 {
		key = args[3]
	}

	manifestPath := ""
	if len(args) > 4 {
		manifestPath = args[4]
	} else {
		manifestPathInfo, err := os.Stat("manifest.yaml")
		if err == nil {
			manifestPath = manifestPathInfo.Name()
		} else if errors.Is(err, os.ErrNotExist) {
			potentialPath := fmt.Sprintf("%s/manifest.yaml", args[4])
			manifestPathInfo, err = os.Stat(potentialPath)
			if errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("reading manifest inputed: %w", err)
			}
		}

	}

	zlog.Info("decoding module",
		zap.String("module_name", moduleName),
		zap.String("store_url", storeUrl),
		zap.Uint64("block_number", blockNumber),
		zap.Uint64("save_internal", saveInterval),
		zap.String("key", key),
		zap.String("manifest_path", manifestPath),
	)

	s, err := dstore.NewStore(storeUrl, "zst", "zstd", false)
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
		return fmt.Errorf("can't find cache at block %d storeUrl %q", blockNumber, moduleStore.BaseURL().String())
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
