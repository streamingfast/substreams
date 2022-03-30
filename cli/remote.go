package cli

import (
	"errors"
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/streamingfast/bstream"
	"github.com/streamingfast/bstream/stream"
	"github.com/streamingfast/dstore"
	"github.com/streamingfast/substreams"
	"github.com/streamingfast/substreams/decode"
	"github.com/streamingfast/substreams/manifest"
	"github.com/streamingfast/substreams/pipeline"
	"github.com/streamingfast/substreams/state"
)

func init() {
	remoteCmd.Flags().String("rpc-endpoint", "http://localhost:8546", "RPC endpoint of blockchain node")
	remoteCmd.Flags().String("state-store-url", "./localdata", "URL of state store")
	remoteCmd.Flags().String("blocks-store-url", "./localblocks", "URL of blocks store")
	remoteCmd.Flags().String("irr-indexes-url", "./localirr", "URL of blocks store")

	remoteCmd.Flags().Int64P("start-block", "s", -1, "Start block for blockchain firehose")
	remoteCmd.Flags().Int64("stop-block", 0, "Stop block for blockchain firehose")
	remoteCmd.Flags().BoolP("partial", "p", false, "Produce partial stores")

	rootCmd.AddCommand(remoteCmd)
}

// remoteCmd represents the base command when called without any subcommands
var remoteCmd = &cobra.Command{
	Use:          "remote [manifest] [module_name] [block_count]",
	Short:        "Run substreams locally",
	RunE:         runRemote,
	Args:         cobra.ExactArgs(3),
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

	localBlocksPath := viper.GetString("blocks-store-url")
	blocksStore, err := dstore.NewDBinStore(localBlocksPath)
	if err != nil {
		return fmt.Errorf("setting up blocks store: %w", err)
	}

	irrIndexesPath := viper.GetString("irr-indexes-url")
	irrStore, err := dstore.NewStore(irrIndexesPath, "", "", false)
	if err != nil {
		return fmt.Errorf("setting up irr blocks store: %w", err)
	}

	rpcClient, rpcCache, err := substreams.GetRPCClient(viper.GetString("rpc-endpoint"), "./rpc-cache")
	if err != nil {
		return fmt.Errorf("setting up rpc client: %w", err)
	}

	stateStorePath := viper.GetString("state-store-url")
	stateStore, err := dstore.NewStore(stateStorePath, "", "", false)
	if err != nil {
		return fmt.Errorf("setting up store for data: %w", err)
	}

	ioFactory := state.NewStoreFactory(stateStore)

	graph, err := manifest.NewModuleGraph(manifProto.Modules)
	if err != nil {
		return fmt.Errorf("create module graph %w", err)
	}

	startBlockNum := viper.GetInt64("start-block")
	stopBlockNum := viper.GetUint64("stop-block")

	var pipelineOpts []pipeline.Option
	if partialMode := viper.GetBool("partial"); partialMode {
		fmt.Println("Starting pipeline in partial mode...")
		pipelineOpts = append(pipelineOpts, pipeline.WithPartialMode())
	}

	if startBlockNum == -1 {
		newStartBlockNum, err := graph.ModuleStartBlock(outputStreamName)
		if err != nil {
			return fmt.Errorf("getting module start block: %w", err)
		}
		startBlockNum = int64(newStartBlockNum)
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
	pipe := pipeline.New(rpcClient, rpcCache, manifProto, graph, outputStreamName, ProtobufBlockType, ioFactory, pipelineOpts...)

	handler, err := pipe.HandlerFactory(ctx, uint64(startBlockNum), stopBlockNum, returnHandler)
	if err != nil {
		return fmt.Errorf("building pipeline handler: %w", err)
	}

	fmt.Println("Starting firehose from block", startBlockNum)

	hose := stream.New([]dstore.Store{blocksStore}, int64(startBlockNum), handler,
		stream.WithForkableSteps(bstream.StepIrreversible),
		stream.WithIrreversibleBlocksIndex(irrStore, []uint64{10000, 1000, 100}),
	)

	if err := hose.Run(ctx); err != nil {
		if errors.Is(err, io.EOF) {
			return nil
		}
		return fmt.Errorf("running the firehose: %w", err)
	}
	time.Sleep(5 * time.Second)

	return nil
}
