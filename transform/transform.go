package transform

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"

	"github.com/streamingfast/bstream"
	"github.com/streamingfast/bstream/transform"
	"github.com/streamingfast/dstore"
	ethrpc "github.com/streamingfast/eth-go/rpc"
	pbfirehose "github.com/streamingfast/pbgo/sf/firehose/v1"
	"github.com/streamingfast/substreams/manifest"
	pbtransform "github.com/streamingfast/substreams/pb/sf/substreams/transform/v1"
	"github.com/streamingfast/substreams/pipeline"
	ssrpc "github.com/streamingfast/substreams/rpc"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

var MessageName = proto.MessageName(&pbtransform.Transform{})

func TransformFactory(rpcEndpoint, rpcCachePath, stateStorePath, protobufBlockType string, statesSaveInterval uint64) *transform.Factory {

	return &transform.Factory{
		Obj: &pbtransform.Transform{},
		NewFunc: func(message *anypb.Any) (transform.Transform, error) {
			mname := message.MessageName()
			if mname != MessageName {
				return nil, fmt.Errorf("expected type url %q, recevied %q ", MessageName, message.TypeUrl)
			}

			req := &pbtransform.Transform{}
			err := proto.Unmarshal(message.Value, req)
			if err != nil {
				return nil, fmt.Errorf("unexpected unmarshall error: %w", err)
			}

			if req.Manifest == nil {
				return nil, fmt.Errorf("missing manifest in request")
			}

			rpcCacheStore, err := dstore.NewStore(rpcCachePath, "", "", false)
			if err != nil {
				return nil, fmt.Errorf("setting up rpc cache store: %w", err)
			}

			rpcCache := ssrpc.NewCache(context.Background(), rpcCacheStore, 0)
			httpClient := &http.Client{
				Transport: &http.Transport{
					DisableKeepAlives: true, // don't reuse connections
				},
				Timeout: 3 * time.Second,
			}

			rpcClient := ethrpc.NewClient(rpcEndpoint, ethrpc.WithHttpClient(httpClient), ethrpc.WithCache(rpcCache))

			stateStore, err := dstore.NewStore(stateStorePath, "", "", false)
			if err != nil {
				return nil, fmt.Errorf("setting up store for data: %w", err)
			}

			graph, err := manifest.NewModuleGraph(req.Manifest.Modules)
			if err != nil {
				return nil, fmt.Errorf("create module graph %w", err)
			}

			t := &ssTransform{
				pipeline:    pipeline.New(rpcClient, rpcCache, req.Manifest, graph, req.OutputModule, protobufBlockType, stateStore, statesSaveInterval),
				description: req.Manifest.Description,
			}

			return t, nil
		},
	}
}

type ssTransform struct {
	pipeline    *pipeline.Pipeline
	description string
}

func (t *ssTransform) Run(
	ctx context.Context,
	req *pbfirehose.Request,
	getStream transform.StreamGetter,
	output transform.StreamOutput,
) error {
	fmt.Println("inside run with request", req)

	newReq := &pbfirehose.Request{
		StartBlockNum: req.StartBlockNum,
		StopBlockNum:  req.StopBlockNum,
		StartCursor:   req.StartCursor,
		ForkSteps:     []pbfirehose.ForkStep{pbfirehose.ForkStep_STEP_IRREVERSIBLE}, //FIXME ?

		// ...FIXME ?
	}

	returnHandler := func(out *pbsubstreams.Output, step bstream.StepType, cursor *bstream.Cursor) error {
		// FIXME we need to get the block here or the step or something...
		// FIXME: use the same ReturnHandler interface, why not store it in `bstream`, and replace
		// that StreamOutput iface.
		return output(cursor, out.GetValue())
	}

	if req.StartBlockNum < 0 {
		return fmt.Errorf("invalid negative startblock (not handled in substreams): %d", req.StartBlockNum)
		// FIXME we want logger too
		// FIXME start block resolving is an art, it should be handled here
	}

	handlerFactoryStartBlock := uint64(req.StartBlockNum) // FIXME why do we need this ?
	handler, err := t.pipeline.HandlerFactory(ctx, handlerFactoryStartBlock, req.StopBlockNum, returnHandler)
	if err != nil {
		return fmt.Errorf("error building substreams pipeline handler: %w", err)
	}

	st, err := getStream(ctx, handler, newReq, zap.NewNop())
	if err != nil {
		return fmt.Errorf("error getting stream: %w", err)
	}
	if err := st.Run(ctx); err != nil {
		if errors.Is(err, io.EOF) {
			return nil
		}
		return fmt.Errorf("running the firehose stream: %w", err)
	}
	return nil
}

func (t *ssTransform) String() string {
	return t.description
}
