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
	"github.com/streamingfast/substreams/manifest"
	"github.com/streamingfast/substreams/pipeline"
	"github.com/streamingfast/substreams/rpc"
)

func LocalRun(ctx context.Context, config *Config) error {
	if bstream.GetBlockDecoder == nil {
		return fmt.Errorf("cannot run local with a build that didn't include chain-specific decoders, compile from sf-ethereum or use the remote command")
	}

	manif, err := manifest.New(config.ManifestPath)
	if err != nil {
		return fmt.Errorf("read manifest %q: %w", config.ManifestPath, err)
	}

	if config.PrintMermaid {
		manif.PrintMermaid()
	}

	manifProto, err := manif.ToProto()
	if err != nil {
		return fmt.Errorf("parse manifest to proto%q: %w", config.ManifestPath, err)
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

	rpcCacheStore, err := dstore.NewStore(config.RpcCacheUrl, "", "", false)
	if err != nil {
		return fmt.Errorf("setting up rpc client: %w", err)
	}

	rpcCache := rpc.NewCacheManager(ctx, rpcCacheStore, int64(config.StartBlock))
	httpClient := &http.Client{
		Transport: &http.Transport{
			DisableKeepAlives: true, // don't reuse connections
		},
		Timeout: 3 * time.Second,
	}

	rpcClient := ethrpc.NewClient(config.RpcEndpoint, ethrpc.WithHttpClient(httpClient), ethrpc.WithCache(rpcCache), ethrpc.WithSecondaryEndpoints(config.SecondaryRpcEndpoints))

	var pipelineOpts []pipeline.Option
	if config.PartialMode {
		fmt.Println("Starting pipeline in partial mode...")
		pipelineOpts = append(pipelineOpts, pipeline.WithPartialMode())
	}
	pipelineOpts = append(pipelineOpts, pipeline.WithAllowInvalidState())

	if config.StartBlock == 0 {
		sb, err := graph.ModuleStartBlock(config.OutputStreamName)
		if err != nil {
			return fmt.Errorf("getting module start block: %w", err)
		}
		config.StartBlock = sb
	}

	pipe := pipeline.New(rpcClient, rpcCache, manifProto, graph, config.OutputStreamName, config.ProtobufBlockType, stateStore, pipelineOpts...)
	handler, err := pipe.HandlerFactory(ctx, config.StartBlock, config.StopBlock, config.ReturnHandler)
	if err != nil {
		return fmt.Errorf("building pipeline handler: %w", err)
	}

	hose := stream.New([]dstore.Store{blocksStore}, int64(config.StartBlock), handler,
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
