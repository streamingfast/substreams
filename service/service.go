package service

import (
	"context"
	"errors"
	"fmt"

	"io"

	"github.com/streamingfast/bstream/stream"
	"github.com/streamingfast/dstore"
	"github.com/streamingfast/firehose"
	firehoseServer "github.com/streamingfast/firehose/server"
	"github.com/streamingfast/logging"
	pbfirehose "github.com/streamingfast/pbgo/sf/firehose/v1"
	"github.com/streamingfast/substreams/manifest"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/pipeline"
	"github.com/streamingfast/substreams/wasm"
	"github.com/streamingfast/substreams/worker"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
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

	logger                 *zap.Logger
	pool                   *worker.Pool
	parallelBlocksRequests int

	grpcClientFactory func() (pbsubstreams.StreamClient, []grpc.CallOption, error)

	parallelSubRequests       int
	blockRangeSizeSubRequests int
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

type Option func(*Service)

func WithWASMExtension(ext wasm.WASMExtensioner) Option {
	return func(s *Service) {
		s.wasmExtensions = append(s.wasmExtensions, ext)
	}
}

func WithPipelineOptions(f pipeline.PipelineOptioner) Option {
	return func(s *Service) {
		s.pipelineOptions = append(s.pipelineOptions, f)
	}
}

func WithPartialMode() Option {
	return func(s *Service) {
		s.partialModeEnabled = true
	}
}

func WithStoresSaveInterval(block uint64) Option {
	return func(s *Service) {
		s.storesSaveInterval = block
	}
}

func WithOutCacheSaveInterval(block uint64) Option {
	return func(s *Service) {
		s.outputCacheSaveBlockInterval = block
	}
}

func WithParallelBlocksRequestsLimit(limit int) Option {
	return func(s *Service) {
		s.parallelBlocksRequests = limit
	}
}

func New(stateStore dstore.Store, blockType string, grpcClientFactory func() (pbsubstreams.StreamClient, []grpc.CallOption, error), parallelSubRequests int, blockRangeSizeSubRequests int, opts ...Option) *Service {
	s := &Service{
		baseStateStore:            stateStore,
		blockType:                 blockType,
		grpcClientFactory:         grpcClientFactory,
		parallelBlocksRequests:    1,
		parallelSubRequests:       parallelSubRequests,
		blockRangeSizeSubRequests: blockRangeSizeSubRequests,
	}

	for _, opt := range opts {
		opt(s)
	}
	s.pool = worker.NewPool(s.parallelBlocksRequests)

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
	_ = logger

	if request.StartBlockNum < 0 {
		return fmt.Errorf("invalid negative startblock (not handled in substreams): %d", request.StartBlockNum)
		// FIXME we want logger too
		// FIXME start block resolving is an art, it should be handled here
	}

	graph, err := manifest.NewModuleGraph(request.Modules.Modules)
	if err != nil {
		return fmt.Errorf("creating module graph %w", err)
	}

	sources := graph.GetSources()
	for _, source := range sources {
		if source != s.blockType {
			return fmt.Errorf("input source not supported. Only %s is accepted", s.blockType)
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

	if md, ok := metadata.FromIncomingContext(ctx); ok {
		partialMode := md.Get("substreams-partial-mode")
		zlog.Debug("extracting meta data", zap.Strings("partial_mode", partialMode))
		if len(partialMode) == 1 && partialMode[0] == "true" {
			// TODO: only allow partial-mode if the AUTHORIZATION layer permits it
			// partial-mode should be
			if !s.partialModeEnabled {
				return status.Error(codes.InvalidArgument, "substreams-partial-mode not enabled on this instance")
			}

			opts = append(opts, pipeline.WithPartialMode(), pipeline.WithAllowInvalidState())
		}
	}

	if s.storesSaveInterval != 0 {
		opts = append(opts, pipeline.WithStoresSaveInterval(s.storesSaveInterval))
	}

	pipe := pipeline.New(ctx, request, graph, s.blockType, s.baseStateStore, s.outputCacheSaveBlockInterval, s.wasmExtensions, s.grpcClientFactory, s.parallelSubRequests, s.blockRangeSizeSubRequests, opts...)

	firehoseReq := &pbfirehose.Request{
		StartBlockNum: request.StartBlockNum,
		StopBlockNum:  request.StopBlockNum,
		StartCursor:   request.StartCursor,
		ForkSteps:     []pbfirehose.ForkStep{pbfirehose.ForkStep_STEP_IRREVERSIBLE}, //FIXME, should we support whatever is supported by the `request` here?

		// ...FIXME ?
	}

	responseHandler := func(resp *pbsubstreams.Response) error {
		if err := streamSrv.Send(resp); err != nil {
			return NewErrSendBlock(err)
		}
		return nil
	}
	handler, err := pipe.HandlerFactory(responseHandler)
	if err != nil {
		return fmt.Errorf("error building substreams pipeline handler: %w", err)
	}

	st, err := s.streamFactory.New(ctx, handler, firehoseReq, zap.NewNop())
	if err != nil {
		return fmt.Errorf("error getting stream: %w", err)
	}
	if err := st.Run(ctx); err != nil {
		if errors.Is(err, io.EOF) {
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
		return status.Errorf(codes.Internal, "unexpected substreams termination: %s", err)
	}
	return nil
}
