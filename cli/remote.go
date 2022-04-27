package cli

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/streamingfast/substreams/client"
	"github.com/streamingfast/substreams/decode"
	"github.com/streamingfast/substreams/manifest"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"google.golang.org/grpc/metadata"
)

func init() {
	remoteCmd.Flags().String("substreams-endpoint", "api.streamingfast.io:443", "Substreams gRPC endpoint")
	remoteCmd.Flags().String("substreams-api-key-envvar", "SUBSTREAMS_API_KEY", "name of variable containing Substreams Authentication token (JWT)")
	remoteCmd.Flags().Int64P("start-block", "s", -1, "Start block for blockchain firehose")
	remoteCmd.Flags().Uint64P("stop-block", "t", 0, "Stop block for blockchain firehose")
	remoteCmd.Flags().StringP("proto-path", "I", "./proto", "Path of proto files")

	remoteCmd.Flags().Bool("no-return-handler", false, "Avoid printing output for module")

	remoteCmd.Flags().BoolP("insecure", "k", false, "Skip certificate validation on GRPC connection")
	remoteCmd.Flags().BoolP("plaintext", "p", false, "Establish GRPC connection in plaintext")
	remoteCmd.Flags().Bool("partial-mode", false, "Request partial processing mode (internal deployments only)")

	rootCmd.AddCommand(remoteCmd)
}

// remoteCmd represents the base command when called without any subcommands
var remoteCmd = &cobra.Command{
	Use:          "remote [manifest] [module_name]",
	Short:        "Run substreams remotely",
	RunE:         runRemote,
	Args:         cobra.ExactArgs(2),
	SilenceUsage: true,
}

func runRemote(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()

	manifestPath := args[0]
	manif, err := manifest.New(manifestPath)
	if err != nil {
		return fmt.Errorf("read manifest %q: %w", manifestPath, err)
	}

	outputStreamNames := strings.Split(args[1], ",")
	returnHandler := func(any *pbsubstreams.BlockScopedData) error { return nil }
	if !mustGetBool(cmd, "no-return-handler") {
		returnHandler = decode.NewPrintReturnHandler(manif, outputStreamNames)
	}

	protoIncludePath := mustGetString(cmd, "proto-path")
	_ = protoIncludePath

	manif.PrintMermaid()

	manifProto, err := manif.ToProto()
	if err != nil {
		return fmt.Errorf("parse manifest to proto%q: %w", manifestPath, err)
	}

	graph, err := manifest.NewModuleGraph(manifProto.Modules)
	if err != nil {
		return fmt.Errorf("create module graph %w", err)
	}

	startBlock := mustGetInt64(cmd, "start-block")
	if startBlock == 0 {
		sb, err := graph.ModuleStartBlock(outputStreamNames[0])
		if err != nil {
			return fmt.Errorf("getting module start block: %w", err)
		}
		startBlock = int64(sb)
	}

	ssClient, callOpts, err := client.NewSubstreamsClient(
		mustGetString(cmd, "substreams-endpoint"),
		os.Getenv(mustGetString(cmd, "substreams-api-key-envvar")),
		mustGetBool(cmd, "insecure"),
		mustGetBool(cmd, "plaintext"),
	)
	if err != nil {
		return fmt.Errorf("substreams client setup: %w", err)
	}

	req := &pbsubstreams.Request{
		StartBlockNum: int64(startBlock),
		StopBlockNum:  mustGetUint64(cmd, "stop-block"),
		ForkSteps:     []pbsubstreams.ForkStep{pbsubstreams.ForkStep_STEP_IRREVERSIBLE},
		Manifest:      manifProto,
		OutputModules: outputStreamNames,
	}

	if mustGetBool(cmd, "partial-mode") {
		ctx = metadata.NewOutgoingContext(ctx, metadata.New(map[string]string{"substreams-partial-mode": "true"}))
	}

	cli, err := ssClient.Blocks(ctx, req, callOpts...)
	if err != nil {
		return fmt.Errorf("call sf.substreams.v1.Stream/Blocks: %w", err)
	}

	for {
		resp, err := cli.Recv()
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}

		switch r := resp.Message.(type) {
		case *pbsubstreams.Response_Progress:
			_ = r.Progress
		case *pbsubstreams.Response_SnapshotData:
			_ = r.SnapshotData
		case *pbsubstreams.Response_SnapshotComplete:
			_ = r.SnapshotComplete
		case *pbsubstreams.Response_Data:
			if err := returnHandler(r.Data); err != nil {
				fmt.Printf("RETURN HANDLER ERROR: %s\n", err)
			}
		}
	}

	return nil
}
