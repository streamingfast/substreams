package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/streamingfast/bstream"
	"github.com/streamingfast/bstream/hub"
	"github.com/streamingfast/bstream/stream"
	dgrpcserver "github.com/streamingfast/dgrpc/server"
	"github.com/streamingfast/dstore"
	"github.com/streamingfast/logging"
	"github.com/streamingfast/substreams"
	"github.com/streamingfast/substreams/client"
	"github.com/streamingfast/substreams/manifest"
	"github.com/streamingfast/substreams/orchestrator"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/pipeline"
	"github.com/streamingfast/substreams/pipeline/outputs"
	"github.com/streamingfast/substreams/wasm"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	otelcode "go.opentelemetry.io/otel/codes"
	ttrace "go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

type Service struct {
	baseStateStore     dstore.Store
	blockType          string // NOTE: can't that be extracted from the actual block messages? with some proto machinery? Was probably useful when `sf.ethereum.codec.v1.Block` didn't correspond to the `sf.ethereum.type.v1.Block` target type.. but that's not true anymore.
	partialModeEnabled bool

	wasmExtensions  []wasm.WASMExtensioner
	pipelineOptions []pipeline.PipelineOptioner

	storesSaveInterval           uint64
	outputCacheSaveBlockInterval uint64

	streamFactory *StreamFactory

	logger *zap.Logger

	workerPool *orchestrator.WorkerPool

	parallelSubRequests       int
	blockRangeSizeSubRequests int

	tracer ttrace.Tracer
}

type StreamFactory struct {
	mergedBlocksStore dstore.Store
	forkedBlocksStore dstore.Store
	hub               *hub.ForkableHub
}

func (sf *StreamFactory) New(
	ctx context.Context,
	h bstream.Handler,
	startBlockNum int64,
	stopBlockNum uint64,
	cursor string,
) (*stream.Stream, error) {

	options := []stream.Option{
		stream.WithStopBlock(stopBlockNum),
		stream.WithCustomStepTypeFilter(bstream.StepsAll), // substreams always wants new, undo, new+irreversible, irreversible, stalled
	}

	if cursor != "" {
		cur, err := bstream.CursorFromOpaque(cursor)
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "invalid start cursor %q: %s", cursor, err)
		}
		options = append(options, stream.WithCursor(cur))
	}

	return stream.New(
		sf.forkedBlocksStore,
		sf.mergedBlocksStore,
		sf.hub,
		startBlockNum,
		h,
		options...), nil
}

func (s *Service) BaseStateStore() dstore.Store {
	return s.baseStateStore
}

func (s *Service) BlockType() string {
	return s.blockType
}

func (s *Service) WasmExtensions() []wasm.WASMExtensioner {
	return s.wasmExtensions
}

func New(
	stateStore dstore.Store,
	blockType string,
	parallelSubRequests int,
	blockRangeSizeSubRequests int,
	substreamsClientConfig *client.SubstreamsClientConfig,
	opts ...Option,
) (s *Service, err error) {
	tracer := otel.GetTracerProvider().Tracer("service")
	s = &Service{
		baseStateStore:            stateStore,
		blockType:                 blockType,
		parallelSubRequests:       parallelSubRequests,
		blockRangeSizeSubRequests: blockRangeSizeSubRequests,
		tracer:                    tracer,
	}
	zlog.Info("creating gprc client factory", zap.Reflect("config", substreamsClientConfig))
	newSubstreamClientFunc := client.NewFactory(substreamsClientConfig)

	s.workerPool = orchestrator.NewWorkerPool(parallelSubRequests, func(tracer ttrace.Tracer) orchestrator.Worker {
		return orchestrator.NewRemoteWorker(newSubstreamClientFunc)
	})

	for _, opt := range opts {
		opt(s)
	}

	return s, nil
}

func (s *Service) Register(
	server dgrpcserver.Server,
	mergedBlocksStore dstore.Store,
	forkedBlocksStore dstore.Store,
	forkableHub *hub.ForkableHub,
	logger *zap.Logger) {

	sf := &StreamFactory{
		mergedBlocksStore: mergedBlocksStore,
		forkedBlocksStore: forkedBlocksStore,
		hub:               forkableHub,
	}

	s.streamFactory = sf
	s.logger = logger
	server.RegisterService(func(gs grpc.ServiceRegistrar) {
		pbsubstreams.RegisterStreamServer(gs, s)
	})
}

func (s *Service) Blocks(request *pbsubstreams.Request, streamSrv pbsubstreams.Stream_BlocksServer) error {
	ctx, span := s.tracer.Start(streamSrv.Context(), "substreams_request")
	span.SetAttributes(attribute.StringSlice("module_outputs", request.OutputModules))
	defer span.End()

	logger := logging.Logger(ctx, s.logger)
	hostname, err := os.Hostname()
	if err != nil {
		logger.Warn("cannot find hostname, using 'unknown'", zap.Error(err))
		hostname = "unknown host"
	}
	if os.Getenv("SUBSTREAMS_SEND_HOSTNAME") == "true" {
		md := metadata.New(map[string]string{"host": hostname})
		err = streamSrv.SetHeader(md)
		if err != nil {
			logger.Warn("cannot send header metadata", zap.Error(err))
		}
	}
	span.SetAttributes(attribute.String("hostname", hostname))

	logger.Info("serving blocks")
	if request.StartBlockNum < 0 {
		// TODO(abourget) start block resolving is an art, it should be handled here
		err := fmt.Errorf("invalid negative startblock (not handled in substreams): %d", request.StartBlockNum)
		span.SetStatus(otelcode.Error, err.Error())
		return err
	}

	if request.Modules == nil {
		err := status.Error(codes.InvalidArgument, "no modules found in request")
		span.SetStatus(otelcode.Error, err.Error())
		return err
	}

	if err := manifest.ValidateModules(request.Modules); err != nil {
		err := status.Error(codes.InvalidArgument, fmt.Sprintf("modules validation failed: %s", err))
		span.SetStatus(otelcode.Error, err.Error())
		return err
	}

	if err := pbsubstreams.ValidateRequest(request); err != nil {
		err := status.Error(codes.InvalidArgument, fmt.Sprintf("validate request: %s", err))
		span.SetStatus(otelcode.Error, err.Error())
		return err
	}

	graph, err := manifest.NewModuleGraph(request.Modules.Modules)
	if err != nil {
		err := fmt.Errorf("creating module graph %w", err)
		span.SetStatus(otelcode.Error, err.Error())
		return err
	}

	sources := graph.GetSources()
	for _, source := range sources {
		if source != s.blockType && source != "sf.substreams.v1.Clock" {
			err := fmt.Errorf(`input source %q not supported, only %q and "sf.substreams.v1.Clock" are valid`, source, s.blockType)
			span.SetStatus(otelcode.Error, err.Error())
			return err
		}
	}

	// TODO: missing dmetering hook that was present for each output
	// payload, we'd send the increment in EgressBytes sent.  We'll
	// want to review that anyway.

	var opts []pipeline.Option
	for _, pipeOpts := range s.pipelineOptions {
		for _, opt := range pipeOpts.PipelineOptions(ctx, request) {
			opts = append(opts, opt)
		}
	}

	/*
		this entire `if` is not good, the ctx is from the StreamServer so there
		is no substreams-partial-mode, the actual flag is substreams-partial-mode-enabled
	*/

	isSubrequest := false
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		partialMode := md.Get("substreams-partial-mode")
		zlog.Debug("extracting meta data", zap.Strings("partial_mode", partialMode))
		if len(partialMode) == 1 && partialMode[0] == "true" {
			// TODO: only allow partial-mode if the AUTHORIZATION layer permits it
			// partial-mode should be
			if !s.partialModeEnabled {
				return status.Error(codes.InvalidArgument, "substreams-partial-mode not enabled on this instance")
			}

			isSubrequest = true
			opts = append(opts, pipeline.WithSubrequestExecution())
		}
	}
	span.SetAttributes(attribute.Bool("sub_request", isSubrequest))

	responseHandler := func(resp *pbsubstreams.Response) error {
		if err := streamSrv.Send(resp); err != nil {
			span.SetStatus(otelcode.Error, err.Error())
			return NewErrSendBlock(err)
		}
		return nil
	}

	pipeTracer := otel.GetTracerProvider().Tracer("pipeline")
	pipe := pipeline.New(ctx, pipeTracer, request, graph, s.blockType, s.baseStateStore, s.storesSaveInterval, s.outputCacheSaveBlockInterval, s.wasmExtensions, s.blockRangeSizeSubRequests, responseHandler, opts...)

	if err := pipe.Init(s.workerPool); err != nil {
		span.SetStatus(otelcode.Error, err.Error())
		return fmt.Errorf("error building pipeline: %w", err)
	}

	zlog.Info("creating firehose stream",
		zap.Int64("start_block", request.StartBlockNum),
		zap.Uint64("end_block", request.StopBlockNum),
	)
	blockStream, err := s.streamFactory.New(
		ctx,
		pipe,
		request.StartBlockNum,
		request.StopBlockNum,
		request.StartCursor,
	)
	if err != nil {
		span.SetStatus(otelcode.Error, err.Error())
		return fmt.Errorf("error getting stream: %w", err)
	}
	if err := blockStream.Run(ctx); err != nil {
		if errors.Is(err, stream.ErrStopBlockReached) {
			logger.Debug("stream of blocks reached end block, triggering StoreSave", zap.Uint64("stop_block_num", request.StopBlockNum))
			if err := pipe.HandleStoreSaveBoundaries(ctx, span, request.StopBlockNum); err != nil { // treat StopBlockNum as possible boundaries (if chain has holes...)
				return err
			}
		}

		if errors.Is(err, io.EOF) || errors.Is(err, stream.ErrStopBlockReached) {
			var d []string
			for _, rng := range pipe.PartialsWritten() {
				d = append(d, fmt.Sprintf("%d-%d", rng.StartBlock, rng.ExclusiveEndBlock))
			}
			partialsWritten := []string{strings.Join(d, ",")}
			zlog.Info("setting trailer", zap.Strings("ranges", partialsWritten))
			streamSrv.SetTrailer(metadata.MD{"substreams-partials-written": partialsWritten})
			span.SetStatus(otelcode.Ok, "")
			return nil
		}

		if errors.Is(err, context.Canceled) {
			span.SetStatus(otelcode.Error, err.Error())
			return status.Error(codes.Canceled, "source canceled")
		}

		if errors.Is(err, context.DeadlineExceeded) {
			span.SetStatus(otelcode.Error, err.Error())
			return status.Error(codes.DeadlineExceeded, "source deadline exceeded")
		}

		var errInvalidArg *stream.ErrInvalidArg
		if errors.As(err, &errInvalidArg) {
			span.SetStatus(otelcode.Error, err.Error())
			return status.Error(codes.InvalidArgument, errInvalidArg.Error())
		}

		var errSendBlock *ErrSendBlock
		if errors.As(err, &errSendBlock) {
			logger.Info("unable to send block probably due to client disconnecting", zap.Error(errSendBlock.inner))
			span.SetStatus(otelcode.Error, err.Error())
			return status.Error(codes.Unavailable, errSendBlock.inner.Error())
		}

		logger.Info("unexpected stream of blocks termination", zap.Error(err))
		span.SetStatus(otelcode.Error, err.Error())
		return status.Errorf(codes.Internal, "unexpected termination: %s", err)
	}
	span.SetStatus(otelcode.Ok, "")
	return nil
}

func sendCachedModuleOutput(ctx context.Context, startBlock, stopBlock uint64, module *pbsubstreams.Module, cache *outputs.OutputCache, responseFunc func(resp *pbsubstreams.Response) error) (lastBlockSent *uint64, err error) {
	cachedRanges, err := cache.ListContinuousCacheRanges(ctx, startBlock)
	if err != nil {
		return nil, fmt.Errorf("listing cached ranges: %w", err)
	}

	zlog.Info("found cached ranges", zap.Int("range_count", len(cachedRanges)))
	for _, r := range cachedRanges {
		//todo: check context
		err := cache.Load(ctx, r)
		if err != nil {
			return nil, fmt.Errorf("loading cache: %w", err)
		}

		for _, item := range cache.SortedCacheItems() {
			//todo: check context
			if item.BlockNum >= stopBlock {
				break
			}

			var output pbsubstreams.ModuleOutputData
			switch module.Kind.(type) {
			case *pbsubstreams.Module_KindMap_:
				output = &pbsubstreams.ModuleOutput_MapOutput{
					MapOutput: &anypb.Any{
						TypeUrl: "type.googleapis.com/" + module.Output.Type,
						Value:   item.Payload,
					},
				}
			case *pbsubstreams.Module_KindStore_:
				deltas := &pbsubstreams.StoreDeltas{}
				err := proto.Unmarshal(item.Payload, deltas)
				if err != nil {
					return nil, fmt.Errorf("unmarshalling output deltas: %w", err)
				}

				output = &pbsubstreams.ModuleOutput_StoreDeltas{
					StoreDeltas: &pbsubstreams.StoreDeltas{Deltas: deltas.Deltas},
				}
			default:
				panic(fmt.Sprintf("invalid module file %T", module.Kind))
			}

			out := &pbsubstreams.BlockScopedData{
				Outputs: []*pbsubstreams.ModuleOutput{
					{
						Name: cache.ModuleName,
						Data: output,
					},
				},
				Clock: &pbsubstreams.Clock{
					Id:        item.BlockID,
					Number:    item.BlockNum,
					Timestamp: item.Timestamp,
				},
				Step:   pbsubstreams.ForkStep_STEP_IRREVERSIBLE,
				Cursor: item.Cursor,
			}

			if err := responseFunc(substreams.NewBlockScopedDataResponse(out)); err != nil {
				return nil, fmt.Errorf("calling return func: %w", err)
			}
			lastBlockSent = &item.BlockNum
		}
		x := r.ExclusiveEndBlock - 1
		lastBlockSent = &x
	}

	return
}
