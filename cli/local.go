package cli

import (
	"errors"
	"fmt"
	"io"
	"strconv"
	"time"

	"google.golang.org/protobuf/types/known/anypb"

	"github.com/spf13/cobra"
	"github.com/streamingfast/bstream"
	"github.com/streamingfast/bstream/stream"
	"github.com/streamingfast/dstore"
	"github.com/streamingfast/substreams/decode"
	"github.com/streamingfast/substreams/manifest"
	"github.com/streamingfast/substreams/pipeline"
	"github.com/streamingfast/substreams/state"
	"github.com/streamingfast/substreams/transform"
)

var ProtobufBlockType string = "sf.ethereum.type.v1.Block"

func init() {
	localCmd.Flags().String("rpc-endpoint", "http://localhost:8546", "RPC endpoint of blockchain node")
	localCmd.Flags().String("state-store-url", "./localdata", "URL of state store")
	localCmd.Flags().String("blocks-store-url", "./localblocks", "URL of blocks store")
	localCmd.Flags().String("irr-indexes-url", "./localirr", "URL of blocks store")

	localCmd.Flags().Int64P("start-block", "s", -1, "Start block for blockchain firehose")
	localCmd.Flags().Uint64P("stop-block", "t", 0, "Stop block for blockchain firehose")
	localCmd.Flags().BoolP("partial", "p", false, "Produce partial stores")
	localCmd.Flags().Bool("no-return-handler", false, "Produce partial stores")

	rootCmd.AddCommand(localCmd)
}

// localCmd represents the base command when called without any subcommands
var localCmd = &cobra.Command{
	Use:          "local [manifest] [module_name] [block_count]",
	Short:        "Run substreams locally",
	RunE:         runLocal,
	Args:         cobra.ExactArgs(3),
	SilenceUsage: true,
}

func runLocal(cmd *cobra.Command, args []string) error {
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

	localBlocksPath := mustGetString(cmd, "blocks-store-url")
	blocksStore, err := dstore.NewDBinStore(localBlocksPath)
	if err != nil {
		return fmt.Errorf("setting up blocks store: %w", err)
	}

	irrIndexesPath := mustGetString(cmd, "irr-indexes-url")
	irrStore, err := dstore.NewStore(irrIndexesPath, "", "", false)
	if err != nil {
		return fmt.Errorf("setting up irr blocks store: %w", err)
	}

	rpcEndpoint := mustGetString(cmd, "rpc-endpoint")
	fmt.Println("ENDPOINT", rpcEndpoint)
	// FIXME: obviously this doesn't belong in `transform`, it's an `eth-centric` thing.
	rpcClient, rpcCache, err := transform.GetRPCClient(rpcEndpoint, "./rpc-cache")
	if err != nil {
		return fmt.Errorf("setting up rpc client: %w", err)
	}

	stateStorePath := mustGetString(cmd, "state-store-url")
	stateStore, err := dstore.NewStore(stateStorePath, "", "", false)
	if err != nil {
		return fmt.Errorf("setting up store for data: %w", err)
	}

	ioFactory := state.NewStoreFactory(stateStore)

	graph, err := manifest.NewModuleGraph(manifProto.Modules)
	if err != nil {
		return fmt.Errorf("create module graph %w", err)
	}

	startBlockNum := mustGetInt64(cmd, "start-block")
	stopBlockNum := mustGetUint64(cmd, "stop-block")

	var pipelineOpts []pipeline.Option
	if partialMode := mustGetBool(cmd, "partial"); partialMode {
		fmt.Println("Starting pipeline in partial mode...")
		pipelineOpts = append(pipelineOpts, pipeline.WithPartialMode())
	}
	pipelineOpts = append(pipelineOpts, pipeline.WithAllowInvalidState())

	if startBlockNum == -1 {
		sb, err := graph.ModuleStartBlock(outputStreamName)
		if err != nil {
			return fmt.Errorf("getting module start block: %w", err)
		}
		startBlockNum = int64(sb)
	}

	if stopBlockNum == 0 {
		var blockCount uint64 = 1000
		if len(args) > 0 {
			val, err := strconv.ParseInt(args[2], 10, 64)
			if err != nil {
				return fmt.Errorf("invalid block count %s", args[2])
			}
			blockCount = uint64(val)
		}

		stopBlockNum = uint64(startBlockNum) + blockCount
	}
	returnHandler := decode.NewPrintReturnHandler(manif, outputStreamName)
	if mustGetBool(cmd, "no-return-handler") {
		returnHandler = func(any *anypb.Any, step bstream.StepType, cursor *bstream.Cursor) error {
			return nil
		}
	}

	pipe := pipeline.New(rpcClient, rpcCache, manifProto, graph, outputStreamName, ProtobufBlockType, ioFactory, pipelineOpts...)

	handler, err := pipe.HandlerFactory(ctx, uint64(startBlockNum), stopBlockNum, returnHandler)
	if err != nil {
		return fmt.Errorf("building pipeline handler: %w", err)
	}

	fmt.Println("Starting firehose stream from block", startBlockNum)

	hose := stream.New([]dstore.Store{blocksStore}, int64(startBlockNum), handler,
		stream.WithForkableSteps(bstream.StepIrreversible),
		stream.WithIrreversibleBlocksIndex(irrStore, []uint64{10000, 1000, 100}),
	)

	if err := hose.Run(ctx); err != nil {
		if errors.Is(err, io.EOF) {
			return nil
		}
		return fmt.Errorf("running the firehose stream: %w", err)
	}
	time.Sleep(5 * time.Second)

	return nil
}
