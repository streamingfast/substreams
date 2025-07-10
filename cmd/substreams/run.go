package main

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/streamingfast/cli/sflags"
	"github.com/streamingfast/substreams/sink"
	"github.com/streamingfast/substreams/tui"
)

func init() {

	// default sinker flags
	sink.AddFlagsToSet(runCmd.Flags(),
		sink.FlagIgnore(sink.FlagDevelopmentMode,
			sink.FlagLiveBlockTimeDelta,
			sink.FlagInfiniteRetry))

	runCmd.Flags().Bool("production-mode", false, "Enable Production Mode, with high-speed parallel processing")
	runCmd.Flags().Uint64("limit-processed-blocks", 10000, "Limit the number of blocks to be processed by the server, including preparing the stores, as a safeguard to prevent unexpected expensive reprocessing (0 disables the limit)")
	runCmd.Flags().StringP("output", "o", "", "Output mode, one of: [tui (and ui), json, jsonl, clock] Defaults to 'tui' when in a TTY is present, and 'json' otherwise")
	runCmd.Flags().StringSlice("debug-modules-initial-snapshot", nil, "List of 'store' modules from which to print the initial data snapshot (Unavailable in Production Mode)")
	runCmd.Flags().StringSlice("debug-modules-output", nil, "List of modules from which to print outputs, deltas and logs, accepts regexes (Unavailable in Production Mode)")

	runCmd.Flags().String("test-file", "", "runs a test file")
	runCmd.Flags().Bool("test-verbose", false, "print out all the results")

	rootCmd.AddCommand(runCmd)
}

// runCmd represents the command to run substreams remotely
var runCmd = &cobra.Command{
	Use:          "run [<manifest> [<module_name>]]",
	Short:        "Stream module to standard output. Use 'substreams gui' for more tools and a better experience.",
	Long:         guiOrRunLongUsage,
	RunE:         runRun,
	Args:         cobra.RangeArgs(1, 2),
	SilenceUsage: true,
}

func runRun(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()

	manifestPath, outputModule, err := ruiOrGuiManifestModulePositionalParams(args)
	if err != nil {
		return err
	}

	endpoint := sflags.MustGetString(cmd, sink.FlagEndpoint)

	sinker, err := sink.NewFromViper(cmd,
		"",
		endpoint,
		manifestPath,
		outputModule,
		"1000000:1000010",
		zlog,
		tracer,
	)
	if err != nil {
		return fmt.Errorf("creating sink: %w", err)
	}

	cursorStr := sflags.MustGetString(cmd, "cursor")
	cursor, err := sink.NewCursor(cursorStr)
	if err != nil {
		return fmt.Errorf("creating cursor: %w", err)
	}

	outputMode := sflags.MustGetString(cmd, "output")
	ui, err := tui.New(endpoint, nil, sinker.Package(), []string{outputModule})
	if err != nil {
		return fmt.Errorf("creating ui: %w", err)
	}

	if err := ui.Init(outputMode); err != nil {
		return fmt.Errorf("TUI initialization: %w", err)
	}
	defer ui.CleanUpTerminal()

	h := sink.NewSinkerFullHandlers(
		ui.HandleBlockScopedData,
		ui.HandleBlockUndoSignal,
		ui.HandleSession,
		ui.HandleProgress,
		ui.HandleDebugSnapshotData,
		ui.HandleDebugSnapshotComplete,
		nil,
	)

	sinker.Run(ctx, cursor, h)
	return sinker.Err()

	//	network := sflags.MustGetString(cmd, "network")
	//	paramsString := sflags.MustGetStringArray(cmd, "params")
	//	params, err := manifest.ParseParams(paramsString)
	//	if err != nil {
	//		return fmt.Errorf("parsing params: %w", err)
	//	}
	//
	//	readerOptions := []manifest.Option{
	//		manifest.WithOverrideNetwork(network),
	//		manifest.WithParams(params),
	//		manifest.WithRegistryURL(getSubstreamsRegistryEndpoint()),
	//	}
	//
	//	protoPath := sflags.MustGetString(cmd, "proto-path")
	//	if protoPath != "" {
	//		readerOptions = append(readerOptions, manifest.WithProtoPath(protoPath))
	//	}
	//
	//	protoDescriptorSet := sflags.MustGetString(cmd, "proto-descriptor-set")
	//	if protoDescriptorSet != "" {
	//		readerOptions = append(readerOptions, manifest.WithProtoDescriptorSet(protoDescriptorSet))
	//	}
	//
	//	if outputModule != "" {
	//		readerOptions = append(readerOptions, manifest.WithOverrideOutputModule(outputModule))
	//	}
	//
	//	if sflags.MustGetBool(cmd, "skip-package-validation") {
	//		readerOptions = append(readerOptions, manifest.SkipPackageValidationReader())
	//	}
	//
	//	manifestReader, err := manifest.NewReader(manifestPath, readerOptions...)
	//	if err != nil {
	//		return fmt.Errorf("manifest reader: %w", err)
	//	}
	//
	//	pkgBundle, err := manifestReader.Read()
	//	if err != nil {
	//		return fmt.Errorf("read manifest %q: %w", manifestPath, err)
	//	}
	//
	//	if pkgBundle == nil {
	//		return fmt.Errorf("no package found")
	//	}
	//
	//	msgDescs, err := manifest.BuildMessageDescriptors(pkgBundle.Package)
	//	if err != nil {
	//		return fmt.Errorf("building message descriptors: %w", err)
	//	}
	//
	//	var testRunner *test.Runner
	//	testFile := sflags.MustGetString(cmd, "test-file")
	//	if testFile != "" {
	//		zlog.Info("running test runner", zap.String(testFile, testFile))
	//		testRunner, err = test.NewRunner(testFile, msgDescs, sflags.MustGetBool(cmd, "test-verbose"), zlog)
	//		if err != nil {
	//			return fmt.Errorf("failed to setup test runner: %w", err)
	//		}
	//	}
	//
	//	productionMode := sflags.MustGetBool(cmd, "production-mode")
	//	debugModulesOutput := sflags.MustGetStringSlice(cmd, "debug-modules-output")
	//	if len(debugModulesOutput) == 0 {
	//		debugModulesOutput = nil
	//	}
	//	if debugModulesOutput != nil && productionMode {
	//		return fmt.Errorf("cannot set 'debug-modules-output' in 'production-mode'")
	//	}
	//
	//	debugModulesInitialSnapshot := sflags.MustGetStringSlice(cmd, "debug-modules-initial-snapshot")
	//	if len(debugModulesInitialSnapshot) == 0 {
	//		debugModulesInitialSnapshot = nil
	//	}
	//
	//	startBlock, readFromModule, err := readStartBlockFlag(cmd, "start-block")
	//	if err != nil {
	//		return fmt.Errorf("stop block: %w", err)
	//	}
	//
	//	if outputModule == "" {
	//		mods, ok := pkgBundle.Graph.TopologicalSort()
	//		if ok {
	//			outputModule = mods[0].Name
	//			fmt.Printf("Selected output module: %s\n", outputModule)
	//		}
	//	}
	//
	//	if readFromModule {
	//		sb, err := pkgBundle.Graph.ModuleInitialBlock(outputModule)
	//		if err != nil {
	//			return fmt.Errorf("getting module start block: %w", err)
	//		}
	//		startBlock = int64(sb)
	//	}
	//
	//	authToken, authType := tools.GetAuth(cmd, "substreams-api-key-envvar", "substreams-api-token-envvar")
	//	substreamsClientConfig := client.NewSubstreamsClientConfig(
	//		endpoint,
	//		authToken,
	//		authType,
	//		sflags.MustGetBool(cmd, "insecure"),
	//		sflags.MustGetBool(cmd, "plaintext"),
	//		"substreams_run",
	//	)
	//
	//	ssClient, connClose, callOpts, headers, err := client.NewSubstreamsClient(substreamsClientConfig)
	//	if err != nil {
	//		return fmt.Errorf("substreams client setup: %w", err)
	//	}
	//	defer connClose()
	//

	//	stopBlock, err := readStopBlockFlag(cmd, startBlock, "stop-block", cursorStr != "")
	//	if err != nil {
	//		return fmt.Errorf("stop block: %w", err)
	//	}
	//
	//	noopMode := sflags.MustGetBool(cmd, "noop-mode")
	//	if noopMode && !productionMode {
	//		zlog.Warn("noop-mode used without production-mode: server will execute in development mode without sending the data, this is probably not what //you want")
	//	}
	//	req := &pbsubstreamsrpc.Request{
	//		StartBlockNum:                       startBlock,
	//		StartCursor:                         cursorStr,
	//		StopBlockNum:                        stopBlock,
	//		FinalBlocksOnly:                     sflags.MustGetBool(cmd, "final-blocks-only"),
	//		Modules:                             pkgBundle.Package.Modules,
	//		OutputModule:                        outputModule,
	//		ProductionMode:                      productionMode,
	//		DebugInitialStoreSnapshotForModules: debugModulesInitialSnapshot,
	//		LimitProcessedBlocks:                sflags.MustGetUint64(cmd, "limit-processed-blocks"),
	//		NoopMode:                            noopMode,
	//		DevOutputModules:                    []string{outputModule},
	//	}
	//
	//	if err := req.Validate(); err != nil {
	//		return fmt.Errorf("validate request: %w", err)
	//	}
	//toPrint := debugModulesOutput
	//if toPrint == nil {
	//	toPrint = []string{outputModule}
	//}

	//streamCtx, cancel := context.WithCancel(ctx)
	//ui.OnTerminated(func(err error) {
	//	if err != nil {
	//		fmt.Printf("UI terminated with error %q\n", err)
	//	}

	//	cancel()
	//})
	//defer cancel()

	//// add additional authorization headers
	//if headers.IsSet() {
	//	streamCtx = metadata.AppendToOutgoingContext(streamCtx, headers.ToArray()...)
	//}
	////parse additional-headers flag
	//additionalHeaders := sflags.MustGetStringSlice(cmd, "header")
	//if additionalHeaders != nil {
	//	res := parseHeaders(additionalHeaders)
	//	headerArray := make([]string, 0, len(res)*2)
	//	for k, v := range res {
	//		headerArray = append(headerArray, k, v)
	//	}
	//	streamCtx = metadata.AppendToOutgoingContext(streamCtx, headerArray...)
	//}

	//ui.SetRequest(req)
	//ui.Connecting()
	//cli, err := ssClient.Blocks(streamCtx, req, callOpts...)
	//if err != nil && streamCtx.Err() != context.Canceled {
	//	return fmt.Errorf("call sf.substreams.rpc.v2.Stream/Blocks: %w", err)
	//}
	//ui.Connected()

}
