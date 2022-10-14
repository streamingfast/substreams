package pipeline

import (
	"context"

	"github.com/streamingfast/logging"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	ttrace "go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

var _zlog, _ = logging.PackageLogger("pipe", "github.com/streamingfast/substreams/pipeline")

type RequestContext struct {
	context.Context

	request      *pbsubstreams.Request
	isSubRequest bool
	logger       *zap.Logger
}

func NewRequestContext(ctx context.Context, req *pbsubstreams.Request, isSubRequest bool) *RequestContext {
	logger := _zlog.With(
		zap.Strings("outputs", req.OutputModules),
		zap.Bool("sub_request", isSubRequest),
		zap.Stringer("trace_id", getTraceID(ctx)),
	)
	return &RequestContext{
		Context:      ctx,
		request:      req,
		isSubRequest: isSubRequest,
		logger:       logger,
	}
}

func (r *RequestContext) Request() *pbsubstreams.Request {
	return r.request
}

func (r *RequestContext) StartBlockNum() uint64 {
	return uint64(r.request.StartBlockNum)
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

func getTraceID(ctx context.Context) (out ttrace.TraceID) {
	return ttrace.SpanFromContext(ctx).SpanContext().TraceID()
}
