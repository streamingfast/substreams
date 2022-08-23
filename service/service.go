package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/streamingfast/bstream/stream"
	"github.com/streamingfast/dstore"
	"github.com/streamingfast/firehose"
	firehoseServer "github.com/streamingfast/firehose/server"
	"github.com/streamingfast/logging"
	pbfirehose "github.com/streamingfast/pbgo/sf/firehose/v2"
	"github.com/streamingfast/substreams"
	"github.com/streamingfast/substreams/manifest"
	"github.com/streamingfast/substreams/orchestrator"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/pipeline"
	"github.com/streamingfast/substreams/pipeline/outputs"
	"github.com/streamingfast/substreams/wasm"
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

	firehoseServer *firehoseServer.Server
	streamFactory  *firehose.StreamFactory

	logger *zap.Logger

	grpcClientFactory substreams.GrpcClientFactory

	workerPool *orchestrator.WorkerPool

	parallelSubRequests       int
	blockRangeSizeSubRequests int

	cacheEnabled bool
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

func New(stateStore dstore.Store, blockType string, grpcClientFactory substreams.GrpcClientFactory, parallelSubRequests int, blockRangeSizeSubRequests int, opts ...Option) *Service {
	s := &Service{
		baseStateStore:            stateStore,
		blockType:                 blockType,
		grpcClientFactory:         grpcClientFactory,
		parallelSubRequests:       parallelSubRequests,
		blockRangeSizeSubRequests: blockRangeSizeSubRequests,
		workerPool:                orchestrator.NewWorkerPool(parallelSubRequests, grpcClientFactory),
	}

	for _, opt := range opts {
		opt(s)
	}

	return s
}

func (s *Service) Register(firehoseServer *firehoseServer.Server, streamFactory *firehose.StreamFactory, logger *zap.Logger) {
	s.streamFactory = streamFactory
	s.firehoseServer = firehoseServer
	s.logger = logger
	firehoseServer.Server.RegisterService(func(gs *grpc.Server) {
		pbsubstreams.RegisterStreamServer(gs, s)
	})
}

func (s *Service) Blocks(request *pbsubstreams.Request, streamSrv pbsubstreams.Stream_BlocksServer) error {
	ctx := streamSrv.Context()
	logger := logging.Logger(ctx, s.logger)

	if os.Getenv("SUBSTREAMS_SEND_HOSTNAME") == "true" {
		hostname, err := os.Hostname()
		if err != nil {
			logger.Warn("cannot find hostname, using 'unknown'", zap.Error(err))
			hostname = "unknown host"
		}
		md := metadata.New(map[string]string{"host": hostname})
		err = streamSrv.SetHeader(md)
		if err != nil {
			logger.Warn("cannot send header metadata", zap.Error(err))
		}
	}

	if request.StartBlockNum < 0 {
		// TODO(abourget) start block resolving is an art, it should be handled here
		zlog.Error("invalid negative startblock (not handled in substreams)", zap.Int64("start_block", request.StartBlockNum))
		return fmt.Errorf("invalid negative startblock (not handled in substreams): %d", request.StartBlockNum)
	}

	if request.Modules == nil {
		return status.Error(codes.InvalidArgument, "no modules found in request")
	}

	if err := manifest.ValidateModules(request.Modules); err != nil {
		return status.Error(codes.InvalidArgument, fmt.Sprintf("modules validation failed: %s", err))
	}

	if err := pbsubstreams.ValidateRequest(request); err != nil {
		return status.Error(codes.InvalidArgument, fmt.Sprintf("validate request: %s", err))
	}

	graph, err := manifest.NewModuleGraph(request.Modules.Modules)
	if err != nil {
		return fmt.Errorf("creating module graph %w", err)
	}

	sources := graph.GetSources()
	for _, source := range sources {
		if source != s.blockType && source != "sf.substreams.v1.Clock" {
			return fmt.Errorf(`input source %q not supported, only %q and "sf.substreams.v1.Clock" are valid`, source, s.blockType)
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
		is no substreams-partial-mode, we actually set the partialModeEnabled
		on the service when substreams-partial-mode-enabled is set to true

			if s.partialModeEnabled {
				opts = append(opts, pipeline.WithPartialModeEnabled(true))
			}
	*/

	if s.partialModeEnabled {
		opts = append(opts, pipeline.WithPartialModeEnabled(true))
	}

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

	if s.storesSaveInterval != 0 {
		opts = append(opts, pipeline.WithStoresSaveInterval(s.storesSaveInterval))
	}

	if s.cacheEnabled {
		opts = append(opts, pipeline.WithCacheEnabled(true))
	}

	responseHandler := func(resp *pbsubstreams.Response) error {
		if err := streamSrv.Send(resp); err != nil {
			return NewErrSendBlock(err)
		}
		return nil
	}

	// TODO: check p.cacheEnabled here also if we make this condition back to true
	if false && !isSubrequest && len(request.OutputModules) == 1 && len(request.InitialStoreSnapshotForModules) == 0 {
		moduleName := request.OutputModules[0]
		module, err := graph.Module(moduleName)

		zlog.Info("try to send module output from cached files", zap.String("module_name", moduleName))
		if err != nil {
			return fmt.Errorf("getting module %q from graph: %w", moduleName, err)
		}

		hash := manifest.HashModuleAsString(request.Modules, graph, module)
		moduleCacheStore, err := s.baseStateStore.SubStore(fmt.Sprintf("%s/outputs", hash))
		moduleOutputCache := outputs.NewOutputCache(moduleName, moduleCacheStore, s.outputCacheSaveBlockInterval)

		lastBlockSent, err := sendCachedModuleOutput(ctx, uint64(request.StartBlockNum), request.StopBlockNum, module, moduleOutputCache, responseHandler)
		if err != nil {
			fmt.Println("sending cached module output: %w", err)
		}

		if lastBlockSent != nil && *lastBlockSent >= request.StopBlockNum {
			zlog.Info("sent full requested data from cached output", zap.String("module_name", moduleName), zap.Uint64("last_block_sent", *lastBlockSent))
			return nil // all done
		}

		if lastBlockSent != nil {
			zlog.Info("sent cached data", zap.String("module_name", moduleName), zap.Uint64("last_block_sent", *lastBlockSent))
			// FIXME(abourget): +1 is always smelly, why wouldn't `sendCachedModuleOutput` return a cursor?
			request.StartBlockNum = int64(*lastBlockSent + 1)
		}

	}

	pipe := pipeline.New(ctx, request, graph, s.blockType, s.baseStateStore, s.outputCacheSaveBlockInterval, s.wasmExtensions, s.grpcClientFactory, s.blockRangeSizeSubRequests, responseHandler, opts...)

	firehoseReq := &pbfirehose.Request{
		StartBlockNum:   request.StartBlockNum,
		StopBlockNum:    request.StopBlockNum,
		Cursor:          request.StartCursor,
		FinalBlocksOnly: false,
		// FIXME(abourget), right now, the pbsubstreams.Request has a
		// ForkSteps that we IGNORE. Eventually, we will want to honor
		// it, but ONLY when we are certain that our Pipeline supports
		// reorgs navigation, which is not the case right now.
		// FIXME(abourget): will we also honor the IrreversibilityCondition?
		// perhaps on the day we actually support it in the Firehose :)
	}

	if err := pipe.Init(s.workerPool); err != nil {
		return fmt.Errorf("error building pipeline: %w", err)
	}

	zlog.Info("creating firehose stream",
		zap.Int64("start_block", firehoseReq.StartBlockNum),
		zap.Uint64("end_block", firehoseReq.StopBlockNum),
	)
	blockStream, err := s.streamFactory.New(ctx, pipe, firehoseReq, false, zap.NewNop())
	if err != nil {
		return fmt.Errorf("error getting stream: %w", err)
	}
	if err := blockStream.Run(ctx); err != nil {
		if errors.Is(err, io.EOF) {
			var d []string
			for _, rng := range pipe.PartialsWritten() {
				d = append(d, fmt.Sprintf("%d-%d", rng.StartBlock, rng.ExclusiveEndBlock))
			}
			partialsWritten := []string{strings.Join(d, ",")}
			zlog.Info("setting trailer", zap.Strings("ranges", partialsWritten))
			streamSrv.SetTrailer(metadata.MD{"substreams-partials-written": partialsWritten})
			return nil
		}

		if errors.Is(err, stream.ErrStopBlockReached) {
			logger.Info("stream of blocks reached end block")
			return nil
		}

		if errors.Is(err, context.Canceled) {
			return status.Error(codes.Canceled, "source canceled")
		}

		if errors.Is(err, context.DeadlineExceeded) {
			return status.Error(codes.DeadlineExceeded, "source deadline exceeded")
		}

		var errInvalidArg *stream.ErrInvalidArg
		if errors.As(err, &errInvalidArg) {
			return status.Error(codes.InvalidArgument, errInvalidArg.Error())
		}

		var errSendBlock *ErrSendBlock
		if errors.As(err, &errSendBlock) {
			logger.Info("unable to send block probably due to client disconnecting", zap.Error(errSendBlock.inner))
			return status.Error(codes.Unavailable, errSendBlock.inner.Error())
		}

		logger.Info("unexpected stream of blocks termination", zap.Error(err))
		return status.Errorf(codes.Internal, "unexpected termination: %s", err)
	}
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
