package cli

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/streamingfast/bstream"
	"github.com/streamingfast/bstream/firehose"
	"github.com/streamingfast/dstore"
	"github.com/streamingfast/eth-go/rpc"
	"github.com/streamingfast/substreams/manifest"
	"github.com/streamingfast/substreams/pipeline"
	ssrpc "github.com/streamingfast/substreams/rpc"
	"github.com/streamingfast/substreams/state"
)

var ProtobufBlockType string = "block"

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

	var blockCount uint64 = 1000
	if len(args) > 0 {
		val, err := strconv.ParseInt(args[2], 10, 64)
		if err != nil {
			return fmt.Errorf("invalid block count %s", args[2])
		}
		blockCount = uint64(val)
	}

	modulesStartBlock := int64(manif.StartBlock)

	startBlockNum := viper.GetInt64("start-block")
	if startBlockNum == 0 {
		startBlockNum = modulesStartBlock
	}
	fmt.Println("START BLOCK NUM", startBlockNum)
	forceLoadState := false
	if startBlockNum > modulesStartBlock {
		forceLoadState = true
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

	rpcCacheStore, err := dstore.NewStore("./rpc-cache", "", "", false)
	if err != nil {
		return fmt.Errorf("setting up store for rpc-cache: %w", err)
	}

	httpClient := &http.Client{
		Transport: &http.Transport{
			DisableKeepAlives: true, // don't reuse connections
		},
		Timeout: 3 * time.Second,
	}

	rpcEndpoint := viper.GetString("rpc-endpoint")
	rpcClient := rpc.NewClient(rpcEndpoint, rpc.WithHttpClient(httpClient))
	rpcCache := ssrpc.NewCache(rpcCacheStore, rpcCacheStore, 0, 999)
	rpcCache.Load(ctx)

	fmt.Println("Using RPC endpoint:", rpcEndpoint)

	stateStorePath := viper.GetString("state-store-url")
	stateStore, err := dstore.NewStore(stateStorePath, "", "", false)
	if err != nil {
		return fmt.Errorf("setting up store for data: %w", err)
	}

	ioFactory := state.NewStoreStateIOFactory(stateStore)

	pipe := pipeline.New(uint64(startBlockNum), rpcClient, rpcCache, manif, outputStreamName, ProtobufBlockType)
	if manif.CodeType == "native" {
		if err := pipe.BuildNative(ioFactory, forceLoadState); err != nil {
			return fmt.Errorf("building pipeline: %w", err)
		}
	} else {
		if err := pipe.BuildWASM(ioFactory, forceLoadState); err != nil {
			return fmt.Errorf("building pipeline: %w", err)
		}
	}

	handler := pipe.HandlerFactory(blockCount)

	hose := firehose.New([]dstore.Store{blocksStore}, startBlockNum, handler,
		firehose.WithForkableSteps(bstream.StepIrreversible),
		firehose.WithIrreversibleBlocksIndex(irrStore, true, []uint64{10000, 1000, 100}),
	)

	if err := hose.Run(context.Background()); err != nil {
		return fmt.Errorf("running the firehose: %w", err)
	}
	time.Sleep(5 * time.Second)

	return nil
}
