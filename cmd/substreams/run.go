package main

import (
	"fmt"
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
	runCmd.Flags().StringP("stop-block", "t", "0", "Stop block to end stream at, inclusively.")

	runCmd.Flags().BoolP("insecure", "k", false, "Skip certificate validation on GRPC connection")
	runCmd.Flags().BoolP("plaintext", "p", false, "Establish GRPC connection in plaintext")

	runCmd.Flags().StringP("output", "o", "", "Output mode. Defaults to 'ui' when in a TTY is present, and 'json' otherwise")
	runCmd.Flags().BoolP("initial-snapshots", "i", false, "Fetch an initial snapshot at start block, before continuing processing.")

	rootCmd.AddCommand(runCmd)
}

// runCmd represents the command to run substreams remotely
var runCmd = &cobra.Command{
	Use:          "run <manifest> <module_name>",
	Short:        "Stream modules from a given package on a remote endpoint",
	RunE:         runRun,
	Args:         cobra.ExactArgs(2),
	SilenceUsage: true,
}

func runRun(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()

	outputMode := mustGetString(cmd, "output")

	manifestPath := args[0]
	manifestReader := manifest.NewReader(manifestPath)
	pkg, err := manifestReader.Read()
	if err != nil {
		return fmt.Errorf("read manifest %q: %w", manifestPath, err)
	}

	outputStreamNames := strings.Split(args[1], ",")

	graph, err := manifest.NewModuleGraph(pkg.Modules.Modules)
	if err != nil {
		return fmt.Errorf("creating module graph: %w", err)
	}

	startBlock := mustGetInt64(cmd, "start-block")
	if startBlock == -1 {
		sb, err := graph.ModuleInitialBlock(outputStreamNames[0])
		if err != nil {
			return fmt.Errorf("getting module start block: %w", err)
		}
		startBlock = int64(sb)
	}

	client.SetConfig(client.NewSubstreamsClientConfig(
		mustGetString(cmd, "substreams-endpoint"),
		readAPIToken(cmd, "substreams-api-token-envvar"),
		mustGetBool(cmd, "insecure"),
		mustGetBool(cmd, "plaintext"),
	))

	ssClient, connClose, callOpts, err := client.NewSubstreamsClient()
	if err != nil {
		return fmt.Errorf("substreams client setup: %w", err)
	}
	defer connClose()

	stopBlock, err := readStopBlockFlag(cmd, startBlock, "stop-block")
	if err != nil {
		return fmt.Errorf("stop block: %w", err)
	}

	req := &pbsubstreams.Request{
		StartBlockNum: startBlock,
		StopBlockNum:  stopBlock,
		ForkSteps:     []pbsubstreams.ForkStep{pbsubstreams.ForkStep_STEP_IRREVERSIBLE},
		Modules:       pkg.Modules,
		OutputModules: outputStreamNames,
	}
	if mustGetBool(cmd, "initial-snapshots") {
		for _, modName := range req.OutputModules {
			for _, v := range pkg.Modules.Modules {
				if modName != v.Name {
					continue
				}

				if _, isStore := v.Kind.(*pbsubstreams.Module_KindStore_); isStore {
					req.InitialStoreSnapshotForModules = append(req.InitialStoreSnapshotForModules, modName)
				}
			}
		}
	}

	if err := pbsubstreams.ValidateRequest(req); err != nil {
		return fmt.Errorf("validate request: %w", err)
	}

	ui := tui.New(req, pkg, outputStreamNames)
	if err := ui.Init(outputMode); err != nil {
		return fmt.Errorf("TUI initialization: %w", err)
	}
	defer ui.CleanUpTerminal()

	ui.SetRequest(req)
	ui.Connecting()
	cli, err := ssClient.Blocks(ctx, req, callOpts...)
	if err != nil {
		return fmt.Errorf("call sf.substreams.v1.Stream/Blocks: %w", err)
	}
	ui.Connected()

	for {
		resp, err := cli.Recv()
		if err != nil {
			if err == io.EOF {
				ui.Cancel()
				fmt.Println("all done")
				return nil
			}
			return err
		}

		if err := ui.IncomingMessage(resp); err != nil {
			fmt.Printf("RETURN HANDLER ERROR: %s\n", err)
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
