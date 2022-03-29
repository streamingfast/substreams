package cli

import (
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/streamingfast/bstream"
	"github.com/streamingfast/bstream/firehose"
	"github.com/streamingfast/dstore"
	"github.com/streamingfast/substreams"
	"github.com/streamingfast/substreams/decode"
	"github.com/streamingfast/substreams/manifest"
	"github.com/streamingfast/substreams/pipeline"
	"github.com/streamingfast/substreams/state"
)

var ProtobufBlockType string = "sf.ethereum.type.v1.Block"

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:          "substreams [manifest] [stream_name] [block_count]",
	Short:        "A substreams runner",
	RunE:         runRoot,
	Args:         cobra.ExactArgs(3),
	SilenceUsage: true,
}

func runRoot(cmd *cobra.Command, args []string) error {
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

	startBlockNum := viper.GetUint64("start-block")
	stopBlockNum := viper.GetUint64("stop-block")

	var pipelineOpts []pipeline.Option
	if partialMode := viper.GetBool("partial"); partialMode {
		fmt.Println("Starting pipeline in partial mode...")
		pipelineOpts = append(pipelineOpts, pipeline.WithPartialMode(startBlockNum))
	}

	if startBlockNum == math.MaxUint64 {
		startBlockNum, err = graph.ModuleStartBlock(outputStreamName)
		if err != nil {
			return fmt.Errorf("getting module start block: %w", err)
		}
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

		stopBlockNum = startBlockNum + blockCount
	}

	returnHandler := decode.NewPrintReturnHandler(manif, outputStreamName)
	pipe := pipeline.New(rpcClient, rpcCache, manifProto, graph, outputStreamName, ProtobufBlockType, ioFactory, pipelineOpts...)

	handler, err := pipe.HandlerFactory(ctx, startBlockNum, stopBlockNum, returnHandler)
	if err != nil {
		return fmt.Errorf("building pipeline handler: %w", err)
	}

	hose := firehose.New([]dstore.Store{blocksStore}, int64(startBlockNum), handler,
		firehose.WithForkableSteps(bstream.StepIrreversible),
		firehose.WithIrreversibleBlocksIndex(irrStore, []uint64{10000, 1000, 100}),
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
