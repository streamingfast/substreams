package tools

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/jhump/protoreflect/desc"
	"github.com/jhump/protoreflect/dynamic"
	"github.com/spf13/cobra"
	"github.com/streamingfast/cli"
	"github.com/streamingfast/cli/sflags"
	"github.com/streamingfast/dstore"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"

	"github.com/streamingfast/substreams/block"
	"github.com/streamingfast/substreams/manifest"
	pbssinternal "github.com/streamingfast/substreams/pb/sf/substreams/intern/v2"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/storage/execout"
	"github.com/streamingfast/substreams/storage/index"
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
		map_pools_created gs://[bucket-url-path] 12487090
		uniswap-v3.spkg store_pools gs://[bucket-url-path] 12487090
		dir-with-manifest store_pools gs://[bucket-url-path] 12487090
	`)),
	RunE:         runDecodeOutputsModuleRunE,
	Args:         cobra.RangeArgs(3, 4),
	SilenceUsage: true,
}

var decodeStatesModuleCmd = &cobra.Command{
	Use:   "states <manifest_file> <module_name> <output_url> <block_number> [<key>]",
	Short: "Decode states base 64 encoded bytes to protobuf data structure",
	Long: cli.Dedent(`
		Running the states command only works if the module is a store. If it is a map an error message will be returned
		to the user. The manifest is optional as it will try to find a file named
		'substreams.yaml' in current working directory if nothing entered. You may enter a directory that contains a 'substreams.yaml'
		file in place of '<manifest_file>, or a link to a remote .spkg file, using urls gs://, http(s)://, ipfs://, etc.'.
	`),
	Example: string(cli.ExamplePrefixed("substreams tools decode states", `
		uniswap-v3 store_eth_prices [bucket-url-path] 12487090 token:051cf5178f60e9def5d5a39b2a988a9f914107cb:dprice:eth
		dir-with-manifest store_pools [bucket-url-path] 12487090 pool:c772a65917d5da983b7fc3c9cfbfb53ef01aef7e
		uniswap-v3.spkg store_pools [bucket-url-path] 12487090
	`)),
	RunE:         runDecodeStatesModuleRunE,
	Args:         cobra.RangeArgs(4, 5),
	SilenceUsage: true,
}

var decodeIndexModuleCmd = &cobra.Command{
	Use:   "index [<manifest_file>] <module_name> <output_url> <block_number>",
	Short: "Decode index file and print the key/values",
	Example: string(cli.ExamplePrefixed("substreams tools decode index", `
		map_pools_created gs://[bucket-url-path] 12487090 pool:c772a65917d5da983b7fc3c9cfbfb53ef01aef7e
	`)),
	RunE:         runDecodeIndexModuleRunE,
	Args:         cobra.RangeArgs(3, 4),
	SilenceUsage: true,
}

func init() {
	decodeCmd.PersistentFlags().Uint64("save-interval", 1000, "Save interval (segment size)")
	decodeCmd.PersistentFlags().Bool("use-test-simple-hash", false, "Use the 'simple hashing' function to get module hashes instead of regular hashes, for testing purposes")
	decodeCmd.PersistentFlags().Bool("match-regexp", false, "Use regular expressions to match keys")
	decodeCmd.PersistentFlags().Int("max-keys", 10, "Maximum keys to print when using match-regexp")

	decodeCmd.AddCommand(decodeOutputsModuleCmd)
	decodeCmd.AddCommand(decodeStatesModuleCmd)
	decodeCmd.AddCommand(decodeIndexModuleCmd)

	Cmd.AddCommand(decodeCmd)
}

func runDecodeStatesModuleRunE(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	saveInterval := sflags.MustGetUint64(cmd, "save-interval")
	manifest.TestUseSimpleHash = sflags.MustGetBool(cmd, "use-test-simple-hash")
	matchRegexp := sflags.MustGetBool(cmd, "match-regexp")
	maxKeys := sflags.MustGetInt(cmd, "max-keys")

	manifestPath := args[0]
	moduleName := args[1]
	storeURL := args[2]
	blockNumber, err := strconv.ParseUint(args[3], 10, 64)
	if err != nil {
		return fmt.Errorf("converting blockNumber to uint: %w", err)
	}

	var key string
	if len(args) == 5 {
		key = args[4]
	}

	zlog.Info("decoding module",
		zap.String("manifest_path", manifestPath),
		zap.String("module_name", moduleName),
		zap.String("store_url", storeURL),
		zap.Uint64("block_number", blockNumber),
		zap.Uint64("save_internal", saveInterval),
		zap.String("key", key),
		zap.Bool("regexp", matchRegexp),
	)

	objStore, err := dstore.NewStore(storeURL, "zst", "zstd", false)
	if err != nil {
		return fmt.Errorf("initializing dstore for %q: %w", storeURL, err)
	}

	manifestReader, err := manifest.NewReader(manifestPath, manifest.SkipPackageValidationReader())
	if err != nil {
		return fmt.Errorf("manifest reader: %w", err)
	}

	pkgBundle, err := manifestReader.Read()
	if err != nil {
		return fmt.Errorf("read manifest %q: %w", manifestPath, err)
	}

	if pkgBundle == nil {
		return fmt.Errorf("no package found")
	}

	pkg := pkgBundle.Package
	graph := pkgBundle.Graph

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

	startBlock := blockNumber - blockNumber%saveInterval
	fmt.Printf(`
* Name: %s
* Hash: %s
* Value Type: %s
* Update Policy: %s
* Initial Block: %d
* Loading at block: %d
`,
		matchingModule.Name,
		moduleHash,
		matchingModule.GetKindStore().ValueType,
		matchingModule.GetKindStore().UpdatePolicy.Pretty(),
		matchingModule.InitialBlock,
		startBlock,
	)

	switch matchingModule.Kind.(type) {
	case *pbsubstreams.Module_KindMap_:
		return fmt.Errorf("no states are available for a mapper")
	case *pbsubstreams.Module_KindStore_:
		if matchRegexp {
			return printStateModule(ctx, startBlock, moduleHash, key, maxKeys, matchingModule, objStore, protoFiles)
		}
		return searchStateModule(ctx, startBlock, moduleHash, key, matchingModule, objStore, protoFiles)
	}
	return fmt.Errorf("module has an unknown")
}

func runDecodeIndexModuleRunE(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	saveInterval := sflags.MustGetUint64(cmd, "save-interval")
	manifest.TestUseSimpleHash = sflags.MustGetBool(cmd, "use-test-simple-hash")

	manifestPath := ""
	if len(args) == 4 {
		manifestPath = args[0]
		args = args[1:]
	}

	moduleName := args[0]
	storeURL := args[1]
	blockNumber, err := strconv.ParseUint(args[2], 10, 64)
	if err != nil {
		return fmt.Errorf("converting blockNumber to uint: %w", err)
	}

	zlog.Info("decoding module",
		zap.String("manifest_path", manifestPath),
		zap.String("module_name", moduleName),
		zap.String("store_url", storeURL),
		zap.Uint64("block_number", blockNumber),
		zap.Uint64("save_internal", saveInterval),
	)

	objStore, err := dstore.NewStore(storeURL, "zst", "zstd", false)
	if err != nil {
		return fmt.Errorf("initializing dstore for %q: %w", storeURL, err)
	}

	manifestReader, err := manifest.NewReader(manifestPath, manifest.SkipPackageValidationReader())
	if err != nil {
		return fmt.Errorf("manifest reader: %w", err)
	}

	pkgBundle, err := manifestReader.Read()
	if err != nil {
		return fmt.Errorf("read manifest %q: %w", manifestPath, err)
	}

	if pkgBundle == nil {
		return fmt.Errorf("no package found")
	}

	pkg := pkgBundle.Package
	graph := pkgBundle.Graph

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

	switch matchingModule.Kind.(type) {
	case *pbsubstreams.Module_KindBlockIndex_:
	default:
		return fmt.Errorf("not a block index module")
	}

	endBlock := blockNumber - (blockNumber % saveInterval) + saveInterval

	indexFile, err := index.NewFile(objStore, moduleHash, matchingModule.Name, zlog, block.NewRange(blockNumber, endBlock))
	if err != nil {
		return fmt.Errorf("instantiating index file: %w", err)
	}
	if err := indexFile.Load(ctx); err != nil {
		return fmt.Errorf("loading index file: %w", err)
	}

	indexFile.Print()
	fmt.Printf("done")

	return nil
}

func runDecodeOutputsModuleRunE(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	saveInterval := sflags.MustGetUint64(cmd, "save-interval")
	manifest.TestUseSimpleHash = sflags.MustGetBool(cmd, "use-test-simple-hash")

	manifestPath := ""
	if len(args) == 4 {
		manifestPath = args[0]
		args = args[1:]
	}

	moduleName := args[0]
	storeURL := args[1]

	requestedBlocks, err := block.ParseRange(args[2], saveInterval)
	if err != nil {
		return fmt.Errorf("parsing range %q: %w", args[2], err)
	}

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

	pkgBundle, err := manifestReader.Read()
	if err != nil {
		return fmt.Errorf("read manifest %q: %w", manifestPath, err)
	}

	if pkgBundle == nil {
		return fmt.Errorf("no package found")
	}

	pkg := pkgBundle.Package
	graph := pkgBundle.Graph
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

	startBlock := requestedBlocks.StartBlock - requestedBlocks.StartBlock%saveInterval
	if startBlock < matchingModule.InitialBlock {
		startBlock = matchingModule.InitialBlock
	}

	switch matchingModule.Kind.(type) {
	case *pbsubstreams.Module_KindMap_:
		return searchOutputsModule(ctx, requestedBlocks, startBlock, saveInterval, moduleHash, matchingModule, s, protoFiles)
	case *pbsubstreams.Module_KindStore_:
		return searchOutputsModuleKvOps(ctx, requestedBlocks, startBlock, saveInterval, moduleHash, matchingModule, s)
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
	extendedHash := manifest.ExtendedModuleHash(module, moduleHash)
	modStore, err := execout.NewConfig(module.Name, module.InitialBlock, pbsubstreams.ModuleKindMap, moduleHash, extendedHash, stateStore, zlog)
	if err != nil {
		return fmt.Errorf("execout new config: %w", err)
	}

	moduleStore, err := stateStore.SubStore(moduleHash + "/outputs")
	if err != nil {
		return fmt.Errorf("can't find substore for hash %q: %w", moduleHash, err)
	}

	rng := block.NewRange(startBlock, startBlock-startBlock%saveInterval+saveInterval)

	outputCache, err := modStore.OpenFileReader(ctx, rng)
	if err != nil {
		return fmt.Errorf("can't create file reader for module %q: %w", module.Name, err)
	}
	zlog.Info("loading block from store", zap.Uint64("start_block", startBlock), zap.Stringer("requested_block_range", requestedBlocks))

	for i := requestedBlocks.StartBlock; i < requestedBlocks.ExclusiveEndBlock; i++ {
		payloadBytes, found, err := outputCache.Get(ctx, i)
		if err != nil {
			return fmt.Errorf("getting block %d from cache %s file %s : %w", i, moduleStore.BaseURL(), outputCache.Filename(), err)
		}
		if !found {
			continue
		}

		fmt.Println("Block", i)
		if len(payloadBytes) == 0 {
			continue
		}
		if err := printObject("", module, protoFiles, payloadBytes); err != nil {
			return fmt.Errorf("printing object: %w", err)
		}
	}
	return nil
}

func searchOutputsModuleKvOps(
	ctx context.Context,
	requestedBlocks *block.Range,
	startBlock,
	saveInterval uint64,
	moduleHash string,
	module *pbsubstreams.Module,
	stateStore dstore.Store,
) error {

	extendedHash := manifest.ExtendedModuleHash(module, moduleHash)
	modStore, err := execout.NewConfig(module.Name, module.InitialBlock, pbsubstreams.ModuleKindMap, moduleHash, extendedHash, stateStore, zlog)
	if err != nil {
		return fmt.Errorf("execout new config: %w", err)
	}

	moduleStore, err := stateStore.SubStore(moduleHash + "/outputs")
	if err != nil {
		return fmt.Errorf("can't find substore for hash %q: %w", moduleHash, err)
	}

	rng := block.NewRange(startBlock, startBlock-startBlock%saveInterval+saveInterval)

	outputCache, err := modStore.OpenFileReader(ctx, rng)
	if err != nil {
		return fmt.Errorf("can't create file reader for module %q: %w", module.Name, err)
	}
	zlog.Info("loading block from store", zap.Uint64("start_block", startBlock), zap.Stringer("requested_block_range", requestedBlocks))

	for i := requestedBlocks.StartBlock; i < requestedBlocks.ExclusiveEndBlock; i++ {
		payloadBytes, found, err := outputCache.Get(ctx, i)
		if err != nil {
			return fmt.Errorf("getting block %d from cache %s file %s : %w", i, moduleStore.BaseURL(), outputCache.Filename(), err)
		}
		if !found {
			continue
		}

		fmt.Println("Block", i)
		if len(payloadBytes) == 0 {
			continue
		}
		if err := printKVOps(payloadBytes); err != nil {
			return fmt.Errorf("printing object: %w", err)
		}
	}
	return nil
}

func printStateModule(
	ctx context.Context,
	startBlock uint64,
	moduleHash string,
	fuzzyMatch string,
	max int,
	module *pbsubstreams.Module,
	stateStore dstore.Store,
	protoFiles []*descriptorpb.FileDescriptorProto,
) error {
	config, err := store.NewConfig(module.Name, module.InitialBlock, moduleHash, module.GetKindStore().GetUpdatePolicy(), module.GetKindStore().GetValueType(), stateStore, nil, 0)
	if err != nil {
		return fmt.Errorf("initializing store config module %q: %w", module.Name, err)
	}
	moduleStore := config.NewFullKV(zlog)

	file := store.NewCompleteFileInfo(module.Name, module.InitialBlock, startBlock)
	if err = moduleStore.Load(ctx, file); err != nil {
		return fmt.Errorf("unable to load file: %w", err)
	}

	fmt.Println("* Filename:", stateStore.BaseURL().JoinPath(moduleHash, "states", file.Filename+".zst").String())
	fmt.Println("* Results:")

	var re *regexp.Regexp
	if fuzzyMatch != "" {
		re, err = regexp.Compile(fuzzyMatch)
		if err != nil {
			return fmt.Errorf("invalid regex pattern %q: %w", fuzzyMatch, err)
		}
	}

	count := 0
	matchCount := 0
	moduleStore.Iter(func(key string, value []byte) error {
		count++
		if re == nil || re.MatchString(key) {
			matchCount++
			if matchCount > max {
				return nil
			}
			return printObject("  - "+key, module, protoFiles, value)
		}
		return nil
	})
	if matchCount > max {
		fmt.Println("    ...")
	}
	fmt.Println("Total:", count)
	fmt.Printf("Matching %q: %d\n", fuzzyMatch, matchCount)

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
	config, err := store.NewConfig(module.Name, module.InitialBlock, moduleHash, module.GetKindStore().GetUpdatePolicy(), module.GetKindStore().GetValueType(), stateStore, nil, 0)
	if err != nil {
		return fmt.Errorf("initializing store config module %q: %w", module.Name, err)
	}
	moduleStore := config.NewFullKV(zlog)

	file := store.NewCompleteFileInfo(module.Name, module.InitialBlock, startBlock)
	if err = moduleStore.Load(ctx, file); err != nil {
		return fmt.Errorf("unable to load file: %w", err)
	}

	fmt.Println("filename:", stateStore.BaseURL().JoinPath(moduleHash, "states", file.Filename+".zst").String())

	bytes, found := moduleStore.GetLast(key)
	if !found {
		return fmt.Errorf("no data found for %q", key)
	}
	return printObject("", module, protoFiles, bytes)
}

func printKVOps(data []byte) error {
	kvOps := &pbssinternal.Operations{}
	if err := proto.Unmarshal(data, kvOps); err != nil {
		return fmt.Errorf("unmarshalling kvOps: %w", err)
	}

	asJSON, err := json.Marshal(kvOps)
	if err != nil {
		return fmt.Errorf("marshalling back as json: %w", err)
	}
	fmt.Println(string(asJSON))
	return nil
}

func printObject(key string, module *pbsubstreams.Module, protoFiles []*descriptorpb.FileDescriptorProto, data []byte) error {
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
				if key != "" {
					fmt.Printf("%s: %v\n", key, val)
				} else {
					fmt.Printf("%v\n", val)
				}
				valuePrinted = true
			default:
				return fmt.Errorf("invalid module kind: %q", module.Kind)
			}
		}
	}

	if valuePrinted {
		return nil
	}

	if key != "" {
		fmt.Printf("%s: %v\n", key, string(data))
	} else {
		fmt.Printf("%v\n", string(data))
	}
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
