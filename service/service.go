package service

import (
	"context"
	"fmt"
	"github.com/streamingfast/bstream/hub"
	dgrpcserver "github.com/streamingfast/dgrpc/server"
	"github.com/streamingfast/dstore"
	"github.com/streamingfast/logging"
	"github.com/streamingfast/substreams/client"
	"github.com/streamingfast/substreams/errors"
	"github.com/streamingfast/substreams/orchestrator"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/pipeline"
	"github.com/streamingfast/substreams/pipeline/execout"
	"github.com/streamingfast/substreams/pipeline/execout/cachev1"
	"github.com/streamingfast/substreams/store"
	"github.com/streamingfast/substreams/wasm"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	tracingcode "go.opentelemetry.io/otel/codes"
	ttrace "go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	grpccode "google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"os"
)

type Service struct {
	blockType                 string
	partialModeEnabled        bool
	wasmExtensions            []wasm.WASMExtensioner
	pipelineOptions           []pipeline.PipelineOptioner
	streamFactory             *StreamFactory
	workerPool                *orchestrator.WorkerPool
	parallelSubRequests       int
	blockRangeSizeSubRequests int

	// properties of cache
	storesSaveInterval           uint64
	outputCacheSaveBlockInterval uint64
	baseStateStore               dstore.Store

	tracer ttrace.Tracer
	logger *zap.Logger
}

func New(
	stateStore dstore.Store,
	blockType string,
	parallelSubRequests int,
	blockRangeSizeSubRequests int,
	substreamsClientConfig *client.SubstreamsClientConfig,
	opts ...Option,
) (s *Service, err error) {
	s = &Service{
		baseStateStore:            stateStore,
		blockType:                 blockType,
		parallelSubRequests:       parallelSubRequests,
		blockRangeSizeSubRequests: blockRangeSizeSubRequests,
		tracer:                    otel.GetTracerProvider().Tracer("service"),
	}

	zlog.Info("creating gprc client factory", zap.Reflect("config", substreamsClientConfig))
	newSubstreamClientFunc := client.NewFactory(substreamsClientConfig)

	s.workerPool = orchestrator.NewWorkerPool(parallelSubRequests, func() orchestrator.Worker {
		return orchestrator.NewRemoteWorker(newSubstreamClientFunc)
	})

	for _, opt := range opts {
		opt(s)
	}

	return s, nil
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
	defer span.End()

	// Weird behavior because we want the pipeline to set the logger in the request Context
	logger := logging.Logger(streamSrv.Context(), s.logger)

	hostname := updateStreamHeadersHostname(streamSrv, logger)
	span.SetAttributes(attribute.String("hostname", hostname))

	if grpcError := s.blocks(ctx, request, streamSrv, logger); grpcError != nil {
		span.SetStatus(tracingcode.Error, grpcError.Cause().Error())
		return grpcError.RpcErr()
	}
	span.SetStatus(tracingcode.Ok, "")
	return nil
}

func (s *Service) blocks(ctx context.Context, request *pbsubstreams.Request, streamSrv pbsubstreams.Stream_BlocksServer, logger *zap.Logger) errors.GRPCError {
	logger.Info("validating request")

	graph, err := validateGraph(request, s.blockType)
	if err != nil {
		return errors.NewBasicErr(status.Error(grpccode.InvalidArgument, err.Error()), err)
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
		logger.Debug("extracting meta data", zap.Strings("partial_mode", partialMode))
		if len(partialMode) == 1 && partialMode[0] == "true" {
			// TODO: only allow partial-mode if the AUTHORIZATION layer permits it
			// partial-mode should be
			if !s.partialModeEnabled {
				return errors.NewBasicErr(status.Error(grpccode.InvalidArgument, "substreams-partial-mode not enabled on this instance"), fmt.Errorf("substreams-partial-mode not enabled on this instance"))
			}
			isSubrequest = true
		}
	}

	responseHandler := func(resp *pbsubstreams.Response) error {
		if err := streamSrv.Send(resp); err != nil {
			return errors.NewErrSendBlock(err)
		}
		return nil
	}

	requestCtx := pipeline.NewRequestContext(ctx, request, isSubrequest)
	storeGenerator := pipeline.NewStoreFactory(s.baseStateStore, s.storesSaveInterval)
	storeBoundary := pipeline.NewStoreBoundary(s.storesSaveInterval)
	cachingEngine := execout.NewNoOpCache()
	if s.baseStateStore != nil {
		cachingEngine, err = cachev1.NewEngine(context.Background(), s.outputCacheSaveBlockInterval, s.baseStateStore, requestCtx.Logger())
		if err != nil {
			return errors.NewBasicErr(status.Errorf(grpccode.Internal, "error building caching engine: %s", err), err)
		}
	}

	storeMap := store.NewMap()
	pipe := pipeline.New(
		requestCtx,
		graph,
		s.blockType,
		s.wasmExtensions,
		s.blockRangeSizeSubRequests,
		cachingEngine,
		storeMap,
		storeGenerator,
		storeBoundary,
		responseHandler,
		opts...,
	)

	if err := pipe.Init(s.workerPool); err != nil {
		return errors.NewBasicErr(status.Errorf(grpccode.Internal, "error building pipeline: %s", err), err)
	}

	zlog.Info("creating firehose stream",
		zap.Int64("start_block", request.StartBlockNum),
		zap.Uint64("end_block", request.StopBlockNum),
	)
	blockStream, err := s.streamFactory.New(
		pipe,
		request.StartBlockNum,
		request.StopBlockNum,
		request.StartCursor,
	)
	if err != nil {
		return errors.NewBasicErr(status.Errorf(grpccode.Internal, "error getting stream: %s", err), err)
	}

	if err := blockStream.Run(ctx); err != nil {
		return pipe.StreamEndedWithErr(streamSrv, err)
	}
	return nil
}

func updateStreamHeadersHostname(streamSrv pbsubstreams.Stream_BlocksServer, logger *zap.Logger) string {
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
	return hostname
}
