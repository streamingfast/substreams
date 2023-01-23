package main

import (
	"context"
	"fmt"
	"github.com/streamingfast/cli"
	"github.com/streamingfast/substreams/tools"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/streamingfast/substreams/client"
	"github.com/streamingfast/substreams/manifest"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/tui"
)

func init() {
	runCmd.Flags().StringP("substreams-endpoint", "e", "api.streamingfast.io:443", "Substreams gRPC endpoint")
	runCmd.Flags().String("substreams-api-token-envvar", "SUBSTREAMS_API_TOKEN", "name of variable containing Substreams Authentication token")
	runCmd.Flags().Int64P("start-block", "s", -1, "Start block to stream from. Defaults to -1, which means the initialBlock of the first module you are streaming")
	runCmd.Flags().StringP("cursor", "c", "", "Cursor to stream from. Leave blank for no cursor")
	runCmd.Flags().StringP("stop-block", "t", "0", "Stop block to end stream at, inclusively.")
	runCmd.Flags().BoolP("insecure", "k", false, "Skip certificate validation on GRPC connection")
	runCmd.Flags().BoolP("plaintext", "p", false, "Establish GRPC connection in plaintext")
	runCmd.Flags().StringP("output", "o", "", "Output mode. Defaults to 'ui' when in a TTY is present, and 'json' otherwise")
	runCmd.Flags().StringSlice("debug-modules-initial-snapshot", nil, "List of 'store' modules from which to print the initial data snapshot (Unavailable in Production Mode")
	runCmd.Flags().StringSlice("debug-modules-output", nil, "List of extra modules from which to print outputs, deltas and logs (Unavailable in Production Mode)")
	runCmd.Flags().Bool("production-mode", false, "Enable Production Mode, with high-speed parallel processing")
	rootCmd.AddCommand(runCmd)
}

// runCmd represents the command to run substreams remotely
var runCmd = &cobra.Command{
	Use:   "run [<manifest>] <module_name>",
	Short: "Stream module outputs from a given package on a remote endpoint",
	Long: cli.Dedent(`
		Stream module outputs from a given package on a remote endpoint. The manifest is optional as it will try to find one a file named 
		'substreams.yaml' in current working directory if nothing entered. You may enter a directory that contains a 'substreams.yaml' 
		file in place of '<manifest_file>'.
	`),
	RunE:         runRun,
	Args:         cobra.RangeArgs(1, 2),
	SilenceUsage: true,
}

func runRun(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	outputMode := mustGetString(cmd, "output")

	manifestPath := ""
	var err error
	if len(args) == 2 {
		manifestPath = args[0]
		args = args[1:]
	} else {
		if cli.DirectoryExists(args[0]) || cli.FileExists(args[0]) || strings.Contains(args[0], ".") {
			return fmt.Errorf("parameter entered likely a manifest file, don't forget to include a '<module_name>' in your command")
		}

		// At this point, we assume the user invoked `substreams run <module_name>` so we `ResolveManifestFile` using the empty string since no argument has been passed.
		manifestPath, err = tools.ResolveManifestFile("")
		if err != nil {
			return fmt.Errorf("resolving manifest: %w", err)
		}
	}

	manifestReader := manifest.NewReader(manifestPath)
	pkg, err := manifestReader.Read()
	if err != nil {
		return fmt.Errorf("read manifest %q: %w", manifestPath, err)
	}

	productionMode := mustGetBool(cmd, "production-mode")
	debugModulesOutput := mustGetStringSlice(cmd, "debug-modules-output")
	if debugModulesOutput != nil && productionMode {
		return fmt.Errorf("cannot set 'debug-modules-output' in 'production-mode'")
	}

	debugModulesInitialSnapshot := mustGetStringSlice(cmd, "debug-modules-initial-snapshot")

	graph, err := manifest.NewModuleGraph(pkg.Modules.Modules)
	if err != nil {
		return fmt.Errorf("creating module graph: %w", err)
	}

	outputModule := args[0]
	startBlock := mustGetInt64(cmd, "start-block")
	if startBlock == -1 {
		sb, err := graph.ModuleInitialBlock(outputModule)
		if err != nil {
			return fmt.Errorf("getting module start block: %w", err)
		}
		startBlock = int64(sb)
	}

	substreamsClientConfig := client.NewSubstreamsClientConfig(
		mustGetString(cmd, "substreams-endpoint"),
		readAPIToken(cmd, "substreams-api-token-envvar"),
		mustGetBool(cmd, "insecure"),
		mustGetBool(cmd, "plaintext"),
	)

	ssClient, connClose, callOpts, err := client.NewSubstreamsClient(substreamsClientConfig)
	if err != nil {
		return fmt.Errorf("substreams client setup: %w", err)
	}
	defer connClose()

	stopBlock, err := readStopBlockFlag(cmd, startBlock, "stop-block")
	if err != nil {
		return fmt.Errorf("stop block: %w", err)
	}

	req := &pbsubstreams.Request{
		StartBlockNum:                       startBlock,
		StartCursor:                         mustGetString(cmd, "cursor"),
		StopBlockNum:                        stopBlock,
		ForkSteps:                           []pbsubstreams.ForkStep{pbsubstreams.ForkStep_STEP_IRREVERSIBLE},
		Modules:                             pkg.Modules,
		OutputModule:                        outputModule,
		OutputModules:                       []string{outputModule}, //added for backwards compatibility, will be removed
		ProductionMode:                      productionMode,
		DebugInitialStoreSnapshotForModules: debugModulesInitialSnapshot,
	}

	if err := pbsubstreams.ValidateRequest(req, false); err != nil {
		return fmt.Errorf("validate request: %w", err)
	}
	toPrint := debugModulesOutput
	if toPrint == nil {
		toPrint = []string{outputModule}
	}

	ui := tui.New(req, pkg, toPrint)
	if err := ui.Init(outputMode); err != nil {
		return fmt.Errorf("TUI initialization: %w", err)
	}
	defer ui.CleanUpTerminal()

	streamCtx, cancel := context.WithCancel(ctx)
	ui.OnTerminated(func(err error) {
		if err != nil {
			fmt.Printf("UI terminated with error %q\n", err)
		}

		cancel()
	})
	defer cancel()

	ui.SetRequest(req)
	ui.Connecting()
	cli, err := ssClient.Blocks(streamCtx, req, callOpts...)
	if err != nil && streamCtx.Err() != context.Canceled {
		return fmt.Errorf("call sf.substreams.v1.Stream/Blocks: %w", err)
	}
	ui.Connected()

	for {
		resp, err := cli.Recv()
		if resp != nil {
			if err := ui.IncomingMessage(resp); err != nil {
				fmt.Printf("RETURN HANDLER ERROR: %s\n", err)
			}
		}
		if err != nil {
			if err == io.EOF {
				ui.Cancel()
				fmt.Println("all done")
				return nil
			}

			// Special handling if interrupted the context ourself, no error
			if streamCtx.Err() == context.Canceled {
				ui.Cancel()
				return nil
			}

			return err
		}
	}
}

func readAPIToken(cmd *cobra.Command, envFlagName string) string {
	envVar := mustGetString(cmd, envFlagName)
	value := os.Getenv(envVar)
	if value != "" {
		return value
	}

	return os.Getenv("SF_API_TOKEN")
}

func readStopBlockFlag(cmd *cobra.Command, startBlock int64, flagName string) (uint64, error) {
	val, err := cmd.Flags().GetString(flagName)
	if err != nil {
		panic(fmt.Sprintf("flags: couldn't find flag %q", flagName))
	}

	isRelative := strings.HasPrefix(val, "+")
	if isRelative {
		if startBlock == -1 {
			return 0, fmt.Errorf("relative end block is supported only with an absolute start block")
		}

		val = strings.TrimPrefix(val, "+")
	}

	endBlock, err := strconv.ParseUint(val, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("end block is invalid: %w", err)
	}

	if isRelative {
		return uint64(startBlock) + endBlock, nil
	}

	return endBlock, nil
}
