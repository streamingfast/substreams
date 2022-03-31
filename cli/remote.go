package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/streamingfast/bstream"
	"github.com/streamingfast/firehose/client"
	pbfirehose "github.com/streamingfast/pbgo/sf/firehose/v1"
	"github.com/streamingfast/substreams/decode"
	"github.com/streamingfast/substreams/manifest"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/transform/v1"
	"google.golang.org/protobuf/types/known/anypb"
)

func init() {
	remoteCmd.Flags().String("firehose-endpoint", "api.streamingfast.io:443", "firehose GRPC endpoint")
	remoteCmd.Flags().String("firehose-api-key-envvar", "FIREHOSE_API_KEY", "name of variable containing firehose authentication token (JWT)")
	remoteCmd.Flags().Int64P("start-block", "s", -1, "Start block for blockchain firehose")
	remoteCmd.Flags().Uint64P("stop-block", "t", 0, "Stop block for blockchain firehose")

	remoteCmd.Flags().BoolP("insecure", "k", false, "Skip certificate validation on GRPC connection")
	remoteCmd.Flags().BoolP("plaintext", "p", false, "Establish GRPC connection in plaintext")

	rootCmd.AddCommand(remoteCmd)
}

// remoteCmd represents the base command when called without any subcommands
var remoteCmd = &cobra.Command{
	Use:          "remote [manifest] [module_name]",
	Short:        "Run substreams locally",
	RunE:         runRemote,
	Args:         cobra.ExactArgs(2),
	SilenceUsage: true,
}

func runRemote(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()

	manifestPath := args[0]
	outputStreamName := args[1]

	manif, err := manifest.New(manifestPath)
	if err != nil {
		return fmt.Errorf("read manifest %q: %w", manifestPath, err)
	}

	manif.PrintMermaid()
	manifProto, err := manif.ToProto()
	if err != nil {
		return fmt.Errorf("parse manifest to proto%q: %w", manifestPath, err)
	}

	sub := &pbsubstreams.Transform{
		OutputModule: outputStreamName,
		Manifest:     manifProto,
	}
	trans, err := anypb.New(sub)
	if err != nil {
		return fmt.Errorf("convert transform to any: %w", err)
	}

	graph, err := manifest.NewModuleGraph(manifProto.Modules)
	if err != nil {
		return fmt.Errorf("create module graph %w", err)
	}

	startBlockNum := mustGetInt64(cmd, "start-block")
	stopBlockNum := mustGetUint64(cmd, "stop-block")

	if startBlockNum == -1 {
		sb, err := graph.ModuleStartBlock(outputStreamName)
		if err != nil {
			return fmt.Errorf("getting module start block: %w", err)
		}
		startBlockNum = int64(sb)
	}
	endpoint := mustGetString(cmd, "firehose-endpoint")
	jwt := os.Getenv(mustGetString(cmd, "firehose-api-key-envvar"))
	insecure := mustGetBool(cmd, "insecure")
	plaintext := mustGetBool(cmd, "plaintext")

	fmt.Println("CALLING ENDPOINT", endpoint)
	fhClient, callOpts, err := client.NewFirehoseClient(endpoint, jwt, insecure, plaintext)
	if err != nil {
		return fmt.Errorf("firehose client: %w", err)
	}

	req := &pbfirehose.Request{
		StartBlockNum: startBlockNum,
		StopBlockNum:  stopBlockNum,
		ForkSteps:     []pbfirehose.ForkStep{pbfirehose.ForkStep_STEP_IRREVERSIBLE},
		Transforms: []*anypb.Any{
			trans,
		},
	}

	cli, err := fhClient.Blocks(ctx, req, callOpts...)
	if err != nil {
		return fmt.Errorf("call Blocks: %w", err)
	}

	returnHandler := decode.NewPrintReturnHandler(manif, outputStreamName)

	for {
		resp, err := cli.Recv()
		if err != nil {
			return err
		}
		cursor, _ := bstream.CursorFromOpaque(resp.Cursor)
		ret := returnHandler(resp.Block, stepFromProto(resp.Step), cursor)
		if ret != nil {
			fmt.Println(ret)
		}

	}

	//	pipe := pipeline.New(rpcClient, rpcCache, manifProto, graph, outputStreamName, ProtobufBlockType, ioFactory, pipelineOpts...)
	//
	//	handler, err := pipe.HandlerFactory(ctx, uint64(startBlockNum), stopBlockNum, returnHandler)
	//	if err != nil {
	//		return fmt.Errorf("building pipeline handler: %w", err)
	//	}
	//
	//	fmt.Println("Starting firehose from block", startBlockNum)
	//
	//	hose := stream.New([]dstore.Store{blocksStore}, int64(startBlockNum), handler,
	//		stream.WithForkableSteps(bstream.StepIrreversible),
	//		stream.WithIrreversibleBlocksIndex(irrStore, []uint64{10000, 1000, 100}),
	//	)
	//
	//	if err := hose.Run(ctx); err != nil {
	//		if errors.Is(err, io.EOF) {
	//			return nil
	//		}
	//		return fmt.Errorf("running the firehose: %w", err)
	//	}
	//	time.Sleep(5 * time.Second)
	//
	// return nil
}

func stepFromProto(step pbfirehose.ForkStep) bstream.StepType {
	switch step {
	case pbfirehose.ForkStep_STEP_NEW:
		return bstream.StepNew
	case pbfirehose.ForkStep_STEP_UNDO:
		return bstream.StepUndo
	case pbfirehose.ForkStep_STEP_IRREVERSIBLE:
		return bstream.StepIrreversible
	}
	return bstream.StepType(0)
}
