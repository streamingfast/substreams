package pipeline

import (
	"context"
	"github.com/streamingfast/logging"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	ttrace "go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

var _zlog, _ = logging.PackageLogger("pipe", "github.com/streamingfast/substreams/pipeline")

type RequestContext struct {
	context.Context

	request      *pbsubstreams.Request
	isSubRequest bool
	traces       []*Trace
	logger       *zap.Logger
}

type Trace struct {
	name string
	ctx  context.Context
	span ttrace.Span
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

func (r *RequestContext) StartSpan(spanName string, tracer ttrace.Tracer, opts ...ttrace.SpanStartOption) ttrace.Span {
	ctx, span := tracer.Start(r, spanName, opts...)
	r.traces = append(r.traces, &Trace{
		name: spanName,
		ctx:  ctx,
		span: span,
	})
	return span
}

func (r *RequestContext) EndSpan(err error) {
	if len(r.traces) == 0 {
		r.logger.Warn("cannot end span, no span started")
		return
	}

	index := len(r.traces) - 1    // Get the index of the top most element.
	element := (r.traces)[index]  // Index into the slice and obtain the element.
	r.traces = (r.traces)[:index] // Remove it from the stack by slicing it off.

	if err == nil {
		element.span.SetStatus(codes.Ok, "")
	} else {
		element.span.SetStatus(codes.Error, err.Error())
	}
	element.span.End()
}

func (r *RequestContext) SetAttributes(kv ...attribute.KeyValue) {
	if len(r.traces) == 0 {
		r.logger.Warn("cannot set attributes, no span started")
		return
	}
	r.traces[len(r.traces)-1].span.SetAttributes(kv...)
}

func (r *RequestContext) AddEvent(name string, options ...ttrace.EventOption) {
	if len(r.traces) == 0 {
		r.logger.Warn("cannot add event , no span started",
			zap.String("name", name),
		)
		return
	}
	r.traces[len(r.traces)-1].span.AddEvent(name, options...)

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
