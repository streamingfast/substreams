package main

import (
	"fmt"
	"github.com/streamingfast/substreams/progress"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/streamingfast/substreams/client"
	"github.com/streamingfast/substreams/decode"
	"github.com/streamingfast/substreams/manifest"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

func init() {
	runCmd.Flags().StringP("substreams-endpoint", "e", "api.streamingfast.io:443", "Substreams gRPC endpoint")
	runCmd.Flags().String("substreams-api-token-envvar", "SUBSTREAMS_API_TOKEN", "name of variable containing Substreams Authentication token (JWT)")
	runCmd.Flags().Int64P("start-block", "s", -1, "Start block for blockchain firehose")
	runCmd.Flags().StringP("stop-block", "t", "0", "Stop block for blockchain firehose")

	runCmd.Flags().BoolP("insecure", "k", false, "Skip certificate validation on GRPC connection")
	runCmd.Flags().BoolP("plaintext", "p", false, "Establish GRPC connection in plaintext")

	runCmd.Flags().BoolP("compact-output", "c", false, "Avoid pretty printing output for module and make it a single compact line")

	rootCmd.AddCommand(runCmd)
	rootCmd.AddCommand(packCmd)
}

// runCmd represents the command to run substreams remotely
var runCmd = &cobra.Command{
	Use:          "run <manifest> <module_name>",
	Short:        "Run substreams remotely",
	RunE:         runRun,
	Args:         cobra.ExactArgs(2),
	SilenceUsage: true,
}

func runRun(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()

	manifestPath := args[0]
	manifestReader := manifest.NewReader(manifestPath)
	pkg, err := manifestReader.Read()
	if err != nil {
		return fmt.Errorf("read manifest %q: %w", manifestPath, err)
	}

	outputStreamNames := strings.Split(args[1], ",")

	returnHandler := func(in *pbsubstreams.Response) error { return nil }
	moduleProgressBar := &progress.ModuleProgressBar{
		Bars: map[progress.ModuleName]*progress.Bar{},
	}

	if os.Getenv("SUBSTREAMS_NO_RETURN_HANDLER") == "" {
		for _, outputStreamName := range outputStreamNames {
			bar := &progress.Bar{}
			bar.Initialized = false
			moduleProgressBar.Bars[progress.ModuleName(outputStreamName)] = bar
		}

		returnHandler, err = decode.NewPrintReturnHandler(pkg, outputStreamNames, !mustGetBool(cmd, "compact-output"), moduleProgressBar)
		if err != nil {
			return fmt.Errorf("new printer for %q: %w", manifestPath, err)
		}
	}

	graph, err := manifest.NewModuleGraph(pkg.Modules.Modules)
	if err != nil {
		return fmt.Errorf("create module graph %w", err)
	}

	startBlock := mustGetInt64(cmd, "start-block")
	if startBlock == -1 {
		sb, err := graph.ModuleStartBlock(outputStreamNames[0])
		if err != nil {
			return fmt.Errorf("getting module start block: %w", err)
		}
		startBlock = int64(sb)
	}

	ssClient, callOpts, err := client.NewSubstreamsClient(
		mustGetString(cmd, "substreams-endpoint"),
		readAPIToken(cmd, "substreams-api-token-envvar"),
		mustGetBool(cmd, "insecure"),
		mustGetBool(cmd, "plaintext"),
	)
	if err != nil {
		return fmt.Errorf("substreams client setup: %w", err)
	}

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

	zlog.Info("connecting...")
	cli, err := ssClient.Blocks(ctx, req, callOpts...)
	if err != nil {
		return fmt.Errorf("call sf.substreams.v1.Stream/Blocks: %w", err)
	}

	zlog.Info("connected")

	for {
		resp, err := cli.Recv()
		if err != nil {
			if err == io.EOF {
				return nil
			}

			return err
		}

		if err := returnHandler(resp); err != nil {
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
