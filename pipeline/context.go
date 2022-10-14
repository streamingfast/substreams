package pipeline

import (
	"context"

	"github.com/streamingfast/bstream"
	"github.com/streamingfast/logging"
	"github.com/streamingfast/substreams/errors"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/tracing"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var _zlog, _ = logging.PackageLogger("pipe", "github.com/streamingfast/substreams/pipeline")

type RequestContext struct {
	context.Context

	// effectiveStartBlock is the actual start block respecting the cursor's precedence,
	effectiveStartBlock uint64
	request             *pbsubstreams.Request
	isSubRequest        bool
	logger              *zap.Logger
}

func NewRequestContext(ctx context.Context, req *pbsubstreams.Request, isSubRequest bool) (*RequestContext, errors.GRPCError) {
	logger := _zlog.With(
		zap.Strings("outputs", req.OutputModules),
		zap.Bool("sub_request", isSubRequest),
		zap.Stringer("trace_id", tracing.GetTraceID(ctx)),
	)

	effectiveStartBlock, err := resolveStartBlockNum(req)
	if err != nil {
		return nil, err
	}

	return &RequestContext{
		Context:             ctx,
		effectiveStartBlock: effectiveStartBlock,
		request:             req,
		isSubRequest:        isSubRequest,
		logger:              logger,
	}, nil
}

func resolveStartBlockNum(req *pbsubstreams.Request) (uint64, errors.GRPCError) {
	// Should already be validated but we play safe here
	if req.StartBlockNum < 0 {
		return 0, errors.NewBasicErr(status.Error(codes.InvalidArgument, "start block num must be positive"), nil)
	}

	if req.StartCursor == "" {
		return uint64(req.StartBlockNum), nil
	}

	cursor, err := bstream.CursorFromOpaque(req.StartCursor)
	if err != nil {
		return 0, errors.NewBasicErr(status.Errorf(codes.InvalidArgument, "cursor is invalid: %s", err.Error()), err)
	}

	return cursor.Block.Num(), nil
}

func (r *RequestContext) Request() *pbsubstreams.Request {
	return r.request
}

func (r *RequestContext) StartBlockNum() uint64 {
	return r.effectiveStartBlock
}

func (r *RequestContext) StopBlockNum() uint64 {
	return r.request.StopBlockNum
}

func (r *RequestContext) StartCursor() string {
	return r.request.StartCursor
}

func (r *RequestContext) SetLogger(logger *zap.Logger) {
	r.logger = logger
}

func (r *RequestContext) Logger() *zap.Logger {
	return r.logger
}
