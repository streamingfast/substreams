package sink

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/dustin/go-humanize"
	"github.com/streamingfast/bstream"
	"github.com/streamingfast/derr"
	"github.com/streamingfast/dgrpc"
	"github.com/streamingfast/dmetrics"
	"github.com/streamingfast/logging"
	"github.com/streamingfast/shutter"
	"github.com/streamingfast/substreams/client"
	"github.com/streamingfast/substreams/manifest"
	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
	pbsubstreamsrpcv2 "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
	pbsubstreamsrpcv3 "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v3"
	pbsubstreamsrpcv4 "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v4"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	_ "google.golang.org/grpc/experimental"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"
)

// IgnoreOutputModuleType can be used instead of the expected output module type
// when you want to validate this yourself, for example if you accept multiple
// output type(s).
const IgnoreOutputModuleType string = ""

// InferOutputModuleFromPackage can be used instead of the actual module's output name
// and has the effect that output module is extracted directly from the [pbsubstreams.Package]
// via the `SinkModule` field.
const InferOutputModuleFromPackage string = "@!##_InferOutputModuleFromSpkg_##!@"

type Sinker struct {
	*shutter.Shutter
	*SinkerConfig

	// State fields that are modified during operation
	buffer                  *blockDataBuffer
	request                 *pbsubstreamsrpcv3.Request
	stats                   *Stats
	requestActiveStartBlock uint64
}

// New creates a new Sinker instance with the provided parameters.
//
// Deprecated: use NewFromConfig instead which takes a SinkerConfig struct, most options can
// simply be applied to the SinkerConfig struct directly.
func New(
	mode SubstreamsMode,
	noopMode bool,
	pkg *pbsubstreams.Package,
	outputModule *pbsubstreams.Module,
	hash manifest.ModuleHash,
	clientConfig *client.SubstreamsClientConfig,
	logger *zap.Logger,
	tracer logging.Tracer,
	opts ...Option,
) (*Sinker, error) {
	sinker, err := NewFromConfig(&SinkerConfig{
		Mode:             mode,
		NoopMode:         noopMode,
		Pkg:              pkg,
		OutputModule:     outputModule,
		OutputModuleHash: hash,
		ClientConfig:     clientConfig,
		Logger:           logger,
		Tracer:           tracer,
	})
	if err != nil {
		return nil, err
	}

	for _, o := range opts {
		o(sinker)
	}

	return sinker, nil
}

// New creates a new Sinker instance from the provided SinkerConfig.
// This function replaces the previous Options pattern with a more structured
// configuration approach. All configuration is now contained within the
// SinkerConfig struct, making it easier to manage and test.
func NewFromConfig(
	config *SinkerConfig,
) (*Sinker, error) {

	s := &Sinker{
		Shutter:      shutter.New(),
		SinkerConfig: config,
		stats:        newStats(config.Logger),
	}

	// Set up buffer unless final blocks only is configured
	if config.UndoBufferSize > 0 && !config.FinalBlocksOnly {
		s.buffer = newBlockDataBuffer(config.UndoBufferSize)
	}

	if s.FinalBlocksOnly && s.buffer != nil {
		s.Logger.Debug("discarding undo buffer since final blocks only requested")
		s.buffer = nil
	}

	s.Logger.Info("sinker configured",
		zap.Stringer("mode", s.Mode),
		zap.Int("module_count", len(s.Pkg.Modules.Modules)),
		zap.String("output_module_name", s.OutputModuleName()),
		zap.String("output_module_type", s.SinkerConfig.OutputModule.Output.Type),
		zap.String("output_module_hash", s.OutputModuleHash()),
		zap.Stringer("client_config", (*substramsClientStringer)(s.SinkerConfig.ClientConfig)),
		zap.Stringer("buffer", s.buffer),
		zap.Int64("start_block", s.SinkerConfig.StartBlock),
		zap.Uint64("stop_block", s.SinkerConfig.StopBlock),
		zap.Int("max_retries", s.MaxRetries),
		zap.Bool("final_blocks_only", s.FinalBlocksOnly),
		zap.Bool("liveness_checker", s.LivenessChecker != nil),
	)

	if s.Mode == SubstreamsModeProduction {
		switch s.OutputModule.Kind.(type) {
		case *pbsubstreams.Module_KindBlockIndex_:
			if config.SupportIndexOutputProductionMode {
				config.Logger.Warn("running sink on an *index module* in *production mode*: until it catches up to LIVE, this sink will display `last_block`'s correct ID in the logs and prometheus metrics for `head_block_drift` will be incorrect")
				s.stats.SetIndexOutputProductionMode()
			} else {
				return nil, errors.New("this sink cannot run in production mode with 'index' module as an output")
			}
		}
	}
	if s.NoopMode {
		s.stats.SetNoop()
	}

	return s, nil
}

type substramsClientStringer client.SubstreamsClientConfig

func (s *substramsClientStringer) String() string {
	config := (*client.SubstreamsClientConfig)(s)

	return fmt.Sprintf("%s (insecure: %t, plaintext: %t, JWT present: %t)", config.Endpoint(), config.Insecure(), config.PlainText(), config.AuthToken() != "")
}

// StartBlock is always defined, defaults to module initial block if not set by the user, 0 if module initial block is not set
// which means start from chain's first streamable block.
func (s *Sinker) StartBlock() int64 {
	return s.SinkerConfig.StartBlock
}

// StopBlock is optional, 0 means run until the chain's head and should be treated as infinite/open-ended
// stream of blocks.
//
// The stop block is considered exclusive, meaning if you set StopBlock to 100, the last block processed
// will be 99.
func (s *Sinker) StopBlock() uint64 {
	return s.SinkerConfig.StopBlock
}

// BlockRange returns a bstream.Range representing the start and stop blocks configured
// in the SinkerConfig. If StopBlock is 0, it returns an open-ended range starting from StartBlock.
func (s *Sinker) BlockRange() *bstream.Range {
	return s.SinkerConfig.BlockRange()
}

func (s *Sinker) Package() *pbsubstreams.Package {
	return s.Pkg
}

func (s *Sinker) BytesRepresentation() BytesRepresentation {
	var network, endpoint string
	if s.Pkg != nil {
		network = s.Pkg.Network
	}
	if s.SinkerConfig.ClientConfig != nil {
		endpoint = s.SinkerConfig.ClientConfig.Endpoint()
	}
	return InferBytesRepresentation(network, endpoint)
}

func (s *Sinker) Request() *pbsubstreamsrpcv3.Request {
	return s.request
}

// OutputModuleHash returns the module output hash, can be used by consumer
// to warn if the module changed between restart of the process.
func (s *Sinker) OutputModuleHash() string {
	return hex.EncodeToString(s.SinkerConfig.OutputModuleHash)
}

func (s *Sinker) OutputModuleName() string {
	return s.SinkerConfig.OutputModule.Name
}

// OutputModuleTypePrefixed returns the prefixed output module's type so the type
// will always be prefixed with "proto:".
func (s *Sinker) OutputModuleTypePrefixed() (prefixed string) {
	_, prefixed = sanitizeModuleType(s.SinkerConfig.OutputModule.Output.Type)
	return
}

// OutputModuleTypeUnprefixed returns the unprefixed output module's type so the type
// will **never** be prefixed with "proto:".
func (s *Sinker) OutputModuleTypeUnprefixed() (unprefixed string) {
	unprefixed, _ = sanitizeModuleType(s.SinkerConfig.OutputModule.Output.Type)
	return
}

// ClientConfig returns the `SubstreamsClientConfig` used by this sinker instance.
func (s *Sinker) ClientConfig() *client.SubstreamsClientConfig {
	return s.SinkerConfig.ClientConfig
}

// EndpointConfig returns the endpoint configuration used by this sinker instance, this is an extraction
// of the endpoint configuration from the client configuration.
func (s *Sinker) EndpointConfig() (endpoint string, plaintext bool, insecure bool) {
	return s.SinkerConfig.ClientConfig.Endpoint(), s.SinkerConfig.ClientConfig.PlainText(), s.SinkerConfig.ClientConfig.Insecure()
}

// ApiToken returns the currently defined ApiToken sets on this sinker instance, ""
// is no api token was configured
func (s *Sinker) ApiToken() string {
	return s.SinkerConfig.ClientConfig.AuthToken()
}

func (s *Sinker) PrintStats() {
	egressBytes := ServerEgressBytes.Get()
	processedBlocks := ProcessedBlocks.Get()
	receivedBlockData := DataMessageCount.Get()

	var noDataReceived string
	if egressBytes == 0 && processedBlocks == 0 {
		noDataReceived = " (no data received)"
	}

	fmt.Fprintf(os.Stderr, "📊 Usage Report%s\n", noDataReceived)
	fmt.Fprintf(os.Stderr, " • Egress Bytes (uncompressed): %s\n", humanize.IBytes(uint64(egressBytes)))
	fmt.Fprintf(os.Stderr, " • Processed Blocks: %s blocks\n", humanize.Comma(int64(processedBlocks)))
	fmt.Fprintf(os.Stderr, " • Received Blocks: %s blocks\n", humanize.Comma(int64(receivedBlockData)))
}

func (s *Sinker) Run(ctx context.Context, cursor *Cursor, handler SinkerHandler) {
	ctx, cancel := context.WithCancel(ctx)
	s.OnTerminating(func(_ error) {
		s.Logger.Info("sinker terminating")
		s.stats.Close()
		cancel()
	})
	s.stats.OnTerminated(func(err error) { s.Shutdown(err) })
	if s.SinkerConfig.PrometheusAddr != "" {
		Metrics.Register()
		go dmetrics.Serve(s.SinkerConfig.PrometheusAddr)
	}

	logEach := 15 * time.Second
	if s.Logger.Core().Enabled(zap.DebugLevel) {
		logEach = 5 * time.Second
	}

	s.stats.Start(logEach)

	fields := []zap.Field{zap.Duration("stats_refresh_each", logEach)}
	if cursor != nil {
		fields = append(fields, zap.Stringer("restarting_at", cursor.Block()))
	}
	if s.adjustedEndBlock() != 0 {
		fields = append(fields, zap.String("end_at", fmt.Sprintf("#%d", s.adjustedEndBlock()-1)))
	}

	if cursor != nil && cursor.Block().Num() >= s.adjustedEndBlock()-1 {
		s.Logger.Info("No more blocks to process: cursor reached your stop block", zap.Stringer("last_block_seen", cursor.Block()))
		s.Shutdown(nil)
		return
	}

	if cursor != nil {
		fields = append(fields, zap.String("cursor", cursor.String()))
	}

	s.Logger.Info("starting sinker", fields...)

	lastCursor, err := s.run(ctx, cursor, handler)
	if err == nil {
		s.Logger.Info("substreams ended correctly, reached your stop block", zap.Stringer("last_block_seen", lastCursor.Block()))

		if v, ok := handler.(SinkerCompletionHandler); ok {
			s.Logger.Info("substreams handler has completion callback defined, calling it")

			if err := v.HandleBlockRangeCompletion(ctx, lastCursor); err != nil {
				s.Shutdown(fmt.Errorf("sinker completion handler error: %w", err))
				return
			}
		}
	}

	// If the context is canceled and we are here, it we have stop running without any other error, so Shutdown without error,
	// we are not the cause of the error. We still shutdown so Sinker last stats is still printed.
	shutdownErr := err
	if ctx.Err() == context.Canceled {
		shutdownErr = nil
	}

	s.Shutdown(shutdownErr)
}

func (s *Sinker) run(ctx context.Context, cursor *Cursor, handler SinkerHandler) (activeCursor *Cursor, err error) {
	activeCursor = cursor

	conn, connClose, callOpts, headers, err := client.NewSubstreamsClientConn(s.SinkerConfig.ClientConfig)
	if err != nil {
		return activeCursor, fmt.Errorf("new substreams client connection: %w", err)
	}

	s.OnTerminating(func(_ error) { connClose() })

	var headersArray []string

	if len(s.ExtraHeaders) > 0 || headers != nil {
		if headers == nil {
			headers = make(client.Headers)
		}

		for k, v := range parseHeaders(s.ExtraHeaders) {
			headers[k] = v
		}

		headersArray = make([]string, 0, len(headers)*2)
		for k, v := range parseHeaders(s.ExtraHeaders) {
			headersArray = append(headersArray, k, v)
		}

		for k, v := range headers {
			headersArray = append(headersArray, k, v)
		}
	}

	// We will wait at max approximatively 5m before dying
	backOff := s.BackOff
	s.Logger.Debug("configured default backoff", zap.String("back_off", fmt.Sprintf("%#v", backOff)))

	if s.MaxRetries == 0 {
		s.Logger.Debug("configured backoff to stop after 0 retries (no retries)")
		backOff = backoff.WithMaxRetries(backOff, 0)
	} else if s.MaxRetries > 0 {
		s.Logger.Debug("configured backoff to stop after specified retries", zap.Int("max_retries", s.MaxRetries))
		backOff = backoff.WithMaxRetries(backOff, uint64(s.MaxRetries))
	} else {
		s.Logger.Debug("configured backoff for infinite retries")
		// For infinite retries (MaxRetries == -1), don't set MaxRetries on backoff
	}

	backOff = backoff.WithContext(backOff, ctx)

	startBlock := s.SinkerConfig.StartBlock
	stopBlock := s.adjustedEndBlock()
	devOutputModules := s.DevOutputModules
	if devOutputModules == nil && s.Mode == SubstreamsModeDevelopment {
		devOutputModules = []string{s.SinkerConfig.OutputModule.Name} // default behavior is to ask only for the output module
	} else if len(devOutputModules) == 1 && devOutputModules[0] == ".*" && s.Mode == SubstreamsModeDevelopment {
		devOutputModules = nil // ask the server to send everything
	}

	params, err := pbsubstreamsrpcv3.ParamsToMap(s.Params)
	if err != nil {
		return nil, err
	}

	for {

		s.request = &pbsubstreamsrpcv3.Request{
			StartBlockNum:        startBlock,
			StopBlockNum:         stopBlock,
			StartCursor:          activeCursor.String(),
			FinalBlocksOnly:      s.FinalBlocksOnly,
			PartialBlocks:        s.PartialBlocks,
			Package:              s.Pkg,
			OutputModule:         s.SinkerConfig.OutputModule.Name,
			ProductionMode:       s.Mode == SubstreamsModeProduction,
			NoopMode:             s.NoopMode,
			DevOutputModules:     devOutputModules,
			LimitProcessedBlocks: s.LimitProcessedBlocks,
			Params:               params,
			Network:              s.Network,
		}

		s.Logger.Info("sending request", zap.String("start_block", fmt.Sprintf("%d", startBlock)), zap.String("stop_block", fmt.Sprintf("%d", stopBlock)), zap.String("cursor", activeCursor.String()))

		// Add extra headers if set
		streamCtx := ctx
		if len(headersArray) > 0 {
			streamCtx = metadata.AppendToOutgoingContext(streamCtx, headersArray...)
		}

		var receivedDataMessage bool
		activeCursor, receivedDataMessage, err = s.doRequest(streamCtx, activeCursor, s.request, conn, s.ClientConfig().ForceProtocolVersion(), callOpts, handler)

		// If we received at least one message, we must reset the backoff
		if receivedDataMessage {
			backOff.Reset()
		}

		if err != nil {
			if errors.Is(err, io.EOF) {
				// We must assume that receiving an `io.EOF` means the stop block was reached. This is because
				// on network that can skips block number, it's possible that we requested to stop on a block
				// number that is no in the chain meaning we will receive `io.EOF` but the last seen block before
				// it is not our block number, we must have confidence in the Substreams provider to respect the
				// protocol
				return activeCursor, nil
			}

			if ctxErr := ctx.Err(); errors.Is(ctxErr, context.Canceled) {
				s.Logger.Debug("substreams encountered an error but we are currently terminating, ignoring it", zap.Error(err))
				return activeCursor, nil
			}

			// Retryable or not, we increment the error counter in all those cases
			SubstreamsErrorCount.Inc()

			var retryableError *derr.RetryableError
			if errors.As(err, &retryableError) {
				s.Logger.Error("substreams encountered a retryable error", zap.Error(retryableError.Unwrap()))

				sleepFor := backOff.NextBackOff()
				if sleepFor == backoff.Stop {
					return activeCursor, fmt.Errorf("%w: %w", ErrBackOffExpired, retryableError.Unwrap())
				}

				s.Logger.Info("sleeping before re-connecting", zap.Duration("sleep", sleepFor))
				time.Sleep(sleepFor)
			} else {
				// Let's not wrap the error, it's not retryable to user will see directly his own error
				return activeCursor, err
			}
		}
	}
}

// When an undo buffer is used, we most finished +N block later than real
// stop block to ensure we accumulate enough blocks to assert "finality".
func (s *Sinker) adjustedEndBlock() (endBlock uint64) {
	if s.SinkerConfig.StopBlock == 0 {
		return 0
	}

	endBlock = s.SinkerConfig.StopBlock
	if s.buffer != nil {
		adjusted := endBlock + uint64(s.buffer.Capacity())
		s.Logger.Debug("adjusted request end block for buffer", zap.Uint64("initial", endBlock), zap.Uint64("adjusted", adjusted))
		endBlock = adjusted
	}
	return
}

// returns cursor, receivedDataMessage, error
func (s *Sinker) doRequest(
	ctx context.Context,
	activeCursor *Cursor,
	req *pbsubstreamsrpcv3.Request,
	conn *grpc.ClientConn,
	forceProtocolVersion client.ProtocolVersion,
	callOpts []grpc.CallOption,
	handler SinkerHandler,
) (
	*Cursor,
	bool,
	error,
) {
	s.Logger.Debug("launching substreams request", zap.Int64("start_block", req.StartBlockNum), zap.Stringer("cursor", activeCursor))
	receivedDataMessage := false

	var streamV23 grpc.ServerStreamingClient[pbsubstreamsrpc.Response]
	var streamV4 grpc.ServerStreamingClient[pbsubstreamsrpcv4.Response]
	var err error

	ssClientV2 := pbsubstreamsrpcv2.NewStreamClient(conn)
	ssClientV3 := pbsubstreamsrpcv3.NewStreamClient(conn)
	ssClientV4 := pbsubstreamsrpcv4.NewStreamClient(conn)
	var isRunningV2 bool

	switch forceProtocolVersion {
	case client.ProtocolVersionV2:
		reqV2, err := req.ToV2()
		if err != nil {
			return activeCursor, receivedDataMessage, fmt.Errorf("failed to convert request to v2: %w", err)
		}

		streamV23, err = ssClientV2.Blocks(ctx, reqV2, callOpts...)
		if err != nil {
			return activeCursor, receivedDataMessage, retryable(fmt.Errorf("call sf.substreams.rpc.v2.Stream/Blocks: %w", err))
		}
		isRunningV2 = true
		s.Logger.Info("substreams using protocol version v2")
	case client.ProtocolVersionV3:
		streamV23, err = ssClientV3.Blocks(ctx, req, callOpts...)
		if err != nil {
			return activeCursor, receivedDataMessage, retryable(fmt.Errorf("call sf.substreams.rpc.v3.Stream/Blocks: %w", err))
		}
		s.Logger.Info("substreams using protocol version v3")
	default:
		streamV4, err = ssClientV4.Blocks(ctx, req, callOpts...)
		if err != nil {
			return activeCursor, receivedDataMessage, retryable(fmt.Errorf("call sf.substreams.rpc.v4.Stream/Blocks: %w", err))
		}
		s.Logger.Info("substreams using protocol version v4")
	}

	var prevBlockTime time.Time
	var afterReceive time.Time
	var lastMessageWasData bool

	for {
		if s.Tracer.Enabled() {
			s.Logger.Debug("substreams waiting to receive message", zap.Stringer("cursor", activeCursor))
		}

		if lastMessageWasData {
			AvgLocalProcessingTime.AddElapsedTime(afterReceive)
			LocalProcessingTime.SetFloat64(AvgLocalProcessingTime.Average().Seconds())
		}
		lastMessageWasData = false // reset

		beforeReceive := time.Now()

		var respV23 *pbsubstreamsrpc.Response
		var respV4 *pbsubstreamsrpcv4.Response

		if streamV4 != nil {
			respV4, err = streamV4.Recv()
		} else {
			respV23, err = streamV23.Recv()
		}

		if err != nil {
			if errors.Is(err, io.EOF) {
				return activeCursor, receivedDataMessage, err
			}

			if dgrpcError := dgrpc.AsGRPCError(err); dgrpcError != nil {
				switch dgrpcError.Code() {
				case codes.Unimplemented, codes.NotFound:
					if forceProtocolVersion.IsUnset() && !isRunningV2 {
						// fallback logic: if v4 failed, we try v3, then v2
						// currently if v4 fails, it will try v2 directly because streamV23 is used for both v2 and v3
						// and we need to decide which one to try.

						// If we were running V4 (streamV4 != nil) and it failed with Unimplemented, let's try V3
						if streamV4 != nil {
							s.Logger.Info("server does not implement sf.substreams.rpc.v4.Stream/Blocks, trying v3")
							streamV23, err = ssClientV3.Blocks(ctx, req, callOpts...)
							if err != nil {
								// if v3 also fails immediately with unimplemented, we will hit this again and go to v2
								if dgrpcError := dgrpc.AsGRPCError(err); dgrpcError != nil && (dgrpcError.Code() == codes.Unimplemented || dgrpcError.Code() == codes.NotFound) {
									s.Logger.Info("server does not implement sf.substreams.rpc.v3.Stream/Blocks, trying v2")
									isRunningV2 = true
									reqV2, err := req.ToV2()
									if err != nil {
										return activeCursor, receivedDataMessage, fmt.Errorf("failed to convert request to v2: %w", err)
									}
									streamV23, err = ssClientV2.Blocks(ctx, reqV2, callOpts...)
									if err != nil {
										return activeCursor, receivedDataMessage, retryable(fmt.Errorf("call sf.substreams.rpc.v2.Stream/Blocks: %w", err))
									}
									streamV4 = nil
									continue
								}
								return activeCursor, receivedDataMessage, retryable(fmt.Errorf("call sf.substreams.rpc.v3.Stream/Blocks: %w", err))
							}
							streamV4 = nil
							continue
						}

						// Fallback to use v2 if the server does not support v3
						s.Logger.Info("server does not implement sf.substreams.rpc.v3.Stream/Blocks, trying v2")
						isRunningV2 = true
						reqV2, err := req.ToV2()
						if err != nil {
							return activeCursor, receivedDataMessage, fmt.Errorf("failed to convert request to v2: %w", err)
						}
						streamV23, err = ssClientV2.Blocks(ctx, reqV2, callOpts...)
						if err != nil {
							return activeCursor, receivedDataMessage, retryable(fmt.Errorf("call sf.substreams.rpc.v2.Stream/Blocks: %w", err))
						}
						streamV4 = nil
						continue
					}
					return activeCursor, receivedDataMessage, fmt.Errorf("stream auth failure: %w", err)

				case codes.Unauthenticated, codes.PermissionDenied:
					return activeCursor, receivedDataMessage, fmt.Errorf("stream auth failure: %w", err)

				case codes.InvalidArgument:
					return activeCursor, receivedDataMessage, fmt.Errorf("stream invalid: %w", err)

				case codes.FailedPrecondition: // ex: related to limit-processed-blocks
					return activeCursor, receivedDataMessage, err

				case codes.ResourceExhausted:
					if strings.Contains(dgrpcError.Message(), "quota exceeded") { // no more bytes/blocks
						return activeCursor, receivedDataMessage, err
					}
					return activeCursor, receivedDataMessage, retryable(err) // no concurrent stream available
				}
			}

			if eh, ok := handler.(SinkerErrorHandler); ok {
				eh.HandleError(ctx, err)
			}

			return activeCursor, receivedDataMessage, retryable(err)
		}

		var message any
		if respV23 != nil {
			MessageSizeBytes.AddInt(proto.Size(respV23))
			message = respV23.Message
		} else {
			MessageSizeBytes.AddInt(proto.Size(respV4))
			// Mapping v4 messages back to pbsubstreamsrpc.isResponse_Message (which is rpc.v2.isResponse_Message)
			// This works for fields that have the same type in both.
			switch r := respV4.Message.(type) {
			case *pbsubstreamsrpcv4.Response_Session:
				message = &pbsubstreamsrpc.Response_Session{Session: r.Session}
			case *pbsubstreamsrpcv4.Response_Progress:
				message = &pbsubstreamsrpc.Response_Progress{Progress: r.Progress}
			case *pbsubstreamsrpcv4.Response_BlockUndoSignal:
				message = &pbsubstreamsrpc.Response_BlockUndoSignal{BlockUndoSignal: r.BlockUndoSignal}
			case *pbsubstreamsrpcv4.Response_FatalError:
				message = &pbsubstreamsrpc.Response_FatalError{FatalError: r.FatalError}
			case *pbsubstreamsrpcv4.Response_DebugSnapshotData:
				message = &pbsubstreamsrpc.Response_DebugSnapshotData{DebugSnapshotData: r.DebugSnapshotData}
			case *pbsubstreamsrpcv4.Response_DebugSnapshotComplete:
				message = &pbsubstreamsrpc.Response_DebugSnapshotComplete{DebugSnapshotComplete: r.DebugSnapshotComplete}
			case *pbsubstreamsrpcv4.Response_BlockScopedDatas:
				// We don't map this to pbsubstreamsrpc.isResponse_Message because there is no equivalent
				// We'll handle it specially below.
			}
		}

		if respV4 != nil {
			s.Logger.Debug("received V4 response", zap.Any("message", message))
			if r, ok := respV4.Message.(*pbsubstreamsrpcv4.Response_BlockScopedDatas); ok {
				s.Logger.Debug("received block scoped data response", zap.Int("count", len(r.BlockScopedDatas.Items)))
				for idx, item := range r.BlockScopedDatas.Items {
					if activeCursor, receivedDataMessage, err = s.processBlockScopedData(ctx, handler, item, idx == 0, beforeReceive, &prevBlockTime, activeCursor); err != nil {
						return activeCursor, receivedDataMessage, fmt.Errorf("processing block scoped data: %w", err)
					}
					afterReceive = time.Now()
					lastMessageWasData = true
				}
				continue
			}
		}

		switch r := message.(type) {
		case *pbsubstreamsrpc.Response_Progress:
			if ph, ok := handler.(SinkerProgressHandler); ok {
				ph.HandleProgress(ctx, r.Progress)
			}

			msg := r.Progress

			latestEndBlockPerStage := make(map[uint32]uint64)
			jobsPerStage := make(map[uint32]uint64)

			for _, j := range msg.RunningJobs {
				jobEndBlock := j.StartBlock + j.ProgressBlocks
				if prevEndBlock, ok := latestEndBlockPerStage[j.Stage]; !ok || jobEndBlock > prevEndBlock {
					latestEndBlockPerStage[j.Stage] = jobEndBlock
				}
				jobsPerStage[j.Stage]++
			}
			for k, val := range latestEndBlockPerStage {
				ProgressMessageLastBlock.SetUint64(val, stageString(k))
			}
			// Reset running jobs count for all stages first, then set actual values.
			// This ensures stages with no running jobs show 0 instead of stale values.
			for i := range msg.Stages {
				ProgressMessageRunningJobs.SetUint64(0, stageString(uint32(i)))
			}
			for k, val := range jobsPerStage {
				ProgressMessageRunningJobs.SetUint64(val, stageString(k))
			}

			stagesModules := make(map[int][]string)
			for i, stage := range msg.Stages {
				stagesModules[i] = stage.Modules
				for j, r := range stage.CompletedRanges {
					if s.Mode == SubstreamsModeProduction && i == len(msg.Stages)-1 { // last stage in production is a mapper. There may be "completed ranges" below the one that includes our start_block
						if s.requestActiveStartBlock <= r.StartBlock && r.EndBlock >= s.requestActiveStartBlock {
							ProgressMessageLastContiguousBlock.SetUint64(r.EndBlock, stageString(uint32(i)))
						}
					} else {
						if j == 0 {
							ProgressMessageLastContiguousBlock.SetUint64(r.EndBlock, stageString(uint32(i)))
						}
					}
				}
			}

			ProgressMessageCount.Inc()
			// The returned value from the server gives an overview of the current progress and not the delta
			// since the last message. Since the server is the source of truth, we just set the value directly.
			ProcessedBlocks.SetUint64(r.Progress.ProcessedBlocks)
			ProcessedBytes.SetUint64(r.Progress.ProcessedBytes.TotalBytesRead)

			if s.Tracer.Enabled() {
				s.Logger.Debug("received response Progress", zap.Reflect("progress", r))
			}

		case *pbsubstreamsrpc.Response_BlockScopedData:
			if activeCursor, receivedDataMessage, err = s.processBlockScopedData(ctx, handler, r.BlockScopedData, true, beforeReceive, &prevBlockTime, activeCursor); err != nil {
				return activeCursor, receivedDataMessage, err
			}
			afterReceive = time.Now()
			lastMessageWasData = true

		case *pbsubstreamsrpc.Response_BlockUndoSignal:
			undoSignal := r.BlockUndoSignal
			block := bstream.NewBlockRef(undoSignal.LastValidBlock.Id, undoSignal.LastValidBlock.Number)

			if s.Tracer.Enabled() {
				s.Logger.Debug("received response BlockUndoSignal", zap.Stringer("last_valid_block", block), zap.String("last_valid_cursor", undoSignal.LastValidCursor))
			}

			cursor, err := NewCursor(undoSignal.LastValidCursor)
			if err != nil {
				return activeCursor, receivedDataMessage, fmt.Errorf("invalid received cursor, 'bstream' library in here is probably not up to date: %w", err)
			}

			activeCursor = cursor

			// We record our stats before the buffer action, so user sees state of "stream" and not state of buffer
			s.stats.RecordBlock(block)
			UndoMessageCount.Inc()
			HeadBlockNumber.SetUint64(block.Num())
			// We don't have the block time in undo case for now, so we don't change it

			if s.buffer == nil {
				if err := handler.HandleBlockUndoSignal(ctx, r.BlockUndoSignal, activeCursor); err != nil {
					return activeCursor, receivedDataMessage, fmt.Errorf("handle BlockUndoSignal: %w", err)
				}
			} else {
				// In the case of dealing with an undo buffer, it's expected that a fork will never
				// go beyond the first block in the buffer because if it does, `s.buffer.HandleBlockUndoSignal` here
				// returns an error.
				//
				// This means ultimately that we expect to never call the downstream `BlockUndoSignalHandler` function.
				err = s.buffer.HandleBlockUndoSignal(r.BlockUndoSignal)
				if err != nil {
					return activeCursor, receivedDataMessage, fmt.Errorf("buffer undo block: %w", err)
				}
			}

		case *pbsubstreamsrpc.Response_DebugSnapshotData:
			if ss, ok := handler.(SinkerSnapshotHandler); ok {
				if err := ss.HandleInitialSnapshotData(ctx, r.DebugSnapshotData); err != nil {
					return activeCursor, receivedDataMessage, fmt.Errorf("handle initial snapshot data: %w", err)
				}
			} else {
				s.Logger.Warn("received debug snapshot message, there is no reason to receive those here", zap.Reflect("message", r))
			}
		case *pbsubstreamsrpc.Response_DebugSnapshotComplete:
			if ss, ok := handler.(SinkerSnapshotHandler); ok {
				if err := ss.HandleInitialSnapshotComplete(ctx, r.DebugSnapshotComplete); err != nil {
					return activeCursor, receivedDataMessage, fmt.Errorf("handle initial snapshot complete: %w", err)
				}
			} else {
				s.Logger.Warn("received debug snapshot message, there is no reason to receive those here", zap.Reflect("message", r))
			}

		case *pbsubstreamsrpc.Response_Session:
			if err := s.handleSessionInit(ctx, handler, r.Session); err != nil {
				return activeCursor, receivedDataMessage, err
			}

		default:
			s.Logger.Info("received unknown type of message", zap.Reflect("message", r))
			UnknownMessageCount.Inc()
		}
	}
}

// handleSessionInit processes a `Response_Session` message received from the Substreams endpoint.
//
// If the registered handler implements [SinkerSessionInitHandler], its [SinkerSessionInitHandler.HandleSessionInit]
// callback is invoked. We do *not* short-circuit afterwards: the default logging and the
// `s.requestActiveStartBlock` assignment are sinker-internal bookkeeping that must run on every
// `Response_Session` message regardless of whether a custom handler is installed. The
// `requestActiveStartBlock` field is later consumed in the `Response_ModulesProgress` case to identify the
// contiguous completed range covering the user's resolved start block, so it must always be kept up to date.
func (s *Sinker) handleSessionInit(ctx context.Context, handler SinkerHandler, session *pbsubstreamsrpc.SessionInit) error {
	if sh, ok := handler.(SinkerSessionInitHandler); ok {
		if err := sh.HandleSessionInit(ctx, s.request, session); err != nil {
			return fmt.Errorf("handle session init: %w", err)
		}
	}

	s.Logger.Info("session initialized with remote endpoint",
		zap.Uint64("max_parallel_workers", session.MaxParallelWorkers),
		zap.Uint64("linear_handoff_block", session.LinearHandoffBlock),
		zap.Uint64("resolved_start_block", session.ResolvedStartBlock),
		zap.String("trace_id", session.TraceId),
	)
	s.requestActiveStartBlock = session.ResolvedStartBlock

	return nil
}

func (s *Sinker) processBlockScopedData(
	ctx context.Context,
	handler SinkerHandler,
	data *pbsubstreamsrpc.BlockScopedData,
	isFirstItem bool,
	beforeReceive time.Time,
	prevBlockTime *time.Time,
	activeCursor *Cursor,
) (newCursor *Cursor, receivedDataMessage bool, err error) {
	receivedDataMessage = true
	if cursorLivenessChecker, ok := s.LivenessChecker.(*CursorBasedLivenessChecker); ok {
		cursorLivenessChecker.CheckCursor(data.Cursor)
	}

	if isFirstItem {
		AvgBlockWaitTime.AddElapsedTime(beforeReceive)
		BlockWaitTime.SetFloat64(AvgBlockWaitTime.Average().Seconds())
	}

	blockTime := data.Clock.Timestamp.AsTime()
	if !prevBlockTime.IsZero() {
		AvgBlockTimeDelta.AddDuration(blockTime.Sub(*prevBlockTime))
		BlockTimeDelta.SetFloat64(AvgBlockTimeDelta.Average().Seconds())
	}
	*prevBlockTime = blockTime

	block := bstream.NewBlockRef(data.Clock.Id, data.Clock.Number)
	moduleOutput := data.Output

	if s.Tracer.Enabled() {
		s.Logger.Debug("received response BlockScopedData", zap.Stringer("at", block), zap.String("module_name", moduleOutput.Name), zap.Int("payload_bytes", len(moduleOutput.MapOutput.Value)))
	}

	// We record our stats before the buffer action, so user sees state of "stream" and not state of buffer
	s.stats.RecordBlock(block)
	HeadBlockNumber.SetUint64(block.Num())
	HeadBlockTimeDrift.SetBlockTime(data.Clock.Timestamp.AsTime())
	DataMessageCount.Inc()
	ServerEgressBytes.AddInt(proto.Size(data))
	BackprocessingCompletion.SetUint64(1)

	cursor, err := NewCursor(data.Cursor)
	if err != nil {
		return activeCursor, receivedDataMessage, fmt.Errorf("invalid received cursor, 'bstream' library in here is probably not up to date: %w", err)
	}

	activeCursor = cursor

	var dataToProcess []*pbsubstreamsrpc.BlockScopedData
	if s.buffer == nil {
		// No buffering, process directly
		dataToProcess = []*pbsubstreamsrpc.BlockScopedData{data}
	} else {
		var err error
		dataToProcess, err = s.buffer.HandleBlockScopedData(data)
		if err != nil {
			return activeCursor, receivedDataMessage, fmt.Errorf("buffer add block data: %w", err)
		}
	}

	for _, blockScopedData := range dataToProcess {
		currentCursor, err := NewCursor(blockScopedData.Cursor)
		if err != nil {
			return activeCursor, receivedDataMessage, fmt.Errorf("invalid received cursor, 'bstream' library in here is probably not up to date: %w", err)
		}

		var isLive *bool
		if s.LivenessChecker != nil {
			isLive = &blockNotLive
			if s.LivenessChecker.IsLive(blockScopedData.Clock) {
				isLive = &liveBlock
			}
			s.stats.SetLiveness(isLive)
		}

		if err := handler.HandleBlockScopedData(ctx, blockScopedData, isLive, currentCursor); err != nil {
			return activeCursor, receivedDataMessage, fmt.Errorf("handle BlockScopedData message at block %s: %w", block, err)
		}
	}

	return activeCursor, true, nil
}

func stageString(i uint32) string {
	return fmt.Sprintf("stage %d", i)
}

func retryable(err error) error {
	return derr.NewRetryableError(err)
}

var (
	liveBlock    bool = true
	blockNotLive bool = false
)

func parseHeaders(headers []string) map[string]string {
	if headers == nil {
		return nil
	}

	result := make(map[string]string)
	for _, header := range headers {
		parts := strings.Split(header, ":")
		if len(parts) != 2 {
			log.Fatalf("invalid header format: %s", header)
		}
		result[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
	}
	return result
}
