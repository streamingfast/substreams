package runtime

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/streamingfast/bstream"
	"github.com/streamingfast/bstream/stream"
	"github.com/streamingfast/dstore"
	ethrpc "github.com/streamingfast/eth-go/rpc"
	"github.com/streamingfast/substreams"
	"github.com/streamingfast/substreams/manifest"
	"github.com/streamingfast/substreams/pipeline"
	"github.com/streamingfast/substreams/rpc"
)

type Local struct {
	hose *stream.Stream
}

type LocalConfig struct {
	ManifestPath     string
	OutputStreamName string

	BlocksStoreUrl string
	StateStoreUrl  string
	IrrIndexesUrl  string

	ProtobufBlockType string

	StartBlock uint64
	StopBlock  uint64

	RpcEndpoint string

	PrintMermaid bool
	RpcCacheUrl  string
	PartialMode  bool

	ReturnHandler substreams.ReturnFunc
}

func LocalRun(ctx context.Context, config *LocalConfig) error {
	if bstream.GetBlockDecoder == nil {
		return fmt.Errorf("cannot run local with a build that didn't include chain-specific decoders, compile from sf-ethereum or use the remote command")
	}

	manifestPath := config.ManifestPath
	outputStreamName := config.OutputStreamName

	manif, err := manifest.New(manifestPath)
	if err != nil {
		return fmt.Errorf("read manifest %q: %w", manifestPath, err)
	}

	if config.PrintMermaid {
		manif.PrintMermaid()
	}

	manifProto, err := manif.ToProto()
	if err != nil {
		return fmt.Errorf("parse manifest to proto%q: %w", manifestPath, err)
	}

	blocksStore, err := dstore.NewDBinStore(config.BlocksStoreUrl)
	if err != nil {
		return fmt.Errorf("setting up blocks store: %w", err)
	}

	irrStore, err := dstore.NewStore(config.IrrIndexesUrl, "", "", false)
	if err != nil {
		return fmt.Errorf("setting up irr blocks store: %w", err)
	}

	stateStore, err := dstore.NewStore(config.StateStoreUrl, "", "", false)
	if err != nil {
		return fmt.Errorf("setting up store for data: %w", err)
	}

	graph, err := manifest.NewModuleGraph(manifProto.Modules)
	if err != nil {
		return fmt.Errorf("create module graph %w", err)
	}

	startBlockNum := config.StartBlock
	stopBlockNum := config.StopBlock

	rpcEndpoint := config.RpcEndpoint
	rpcCacheURL := config.RpcCacheUrl

	rpcCacheStore, err := dstore.NewStore(rpcCacheURL, "", "", false)
	if err != nil {
		return fmt.Errorf("setting up rpc client: %w", err)
	}

	rpcCache := rpc.NewCacheManager(ctx, rpcCacheStore, int64(startBlockNum))
	httpClient := &http.Client{
		Transport: &http.Transport{
			DisableKeepAlives: true, // don't reuse connections
		},
		Timeout: 3 * time.Second,
	}

	rpcClient := ethrpc.NewClient(rpcEndpoint, ethrpc.WithHttpClient(httpClient), ethrpc.WithCache(rpcCache))

	var pipelineOpts []pipeline.Option
	if config.PartialMode {
		fmt.Println("Starting pipeline in partial mode...")
		pipelineOpts = append(pipelineOpts, pipeline.WithPartialMode())
	}
	pipelineOpts = append(pipelineOpts, pipeline.WithAllowInvalidState())

	if startBlockNum == 0 {
		sb, err := graph.ModuleStartBlock(outputStreamName)
		if err != nil {
			return fmt.Errorf("getting module start block: %w", err)
		}
		startBlockNum = sb
	}

	pipe := pipeline.New(rpcClient, rpcCache, manifProto, graph, outputStreamName, config.ProtobufBlockType, stateStore, pipelineOpts...)
	handler, err := pipe.HandlerFactory(ctx, startBlockNum, stopBlockNum, config.ReturnHandler)
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
