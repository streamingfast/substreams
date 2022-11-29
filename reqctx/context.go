package reqctx

import (
	"context"
	"errors"
	"io"

	"github.com/streamingfast/logging"
	"github.com/streamingfast/substreams/metrics"
	"go.opentelemetry.io/otel/codes"
	ttrace "go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

type contextKeyType int

var detailsKey = contextKeyType(0)
var tracerKey = contextKeyType(2)
var spanKey = contextKeyType(3)
var reqStatsKey = contextKeyType(4)

func Logger(ctx context.Context) *zap.Logger {
	return logging.Logger(ctx, zap.NewNop())
}

var WithLogger = logging.WithLogger

func Tracer(ctx context.Context) ttrace.Tracer {
	tracer := ctx.Value(tracerKey)
	if t, ok := tracer.(ttrace.Tracer); ok {
		return t
	}
	return ttrace.NewNoopTracerProvider().Tracer("")
}

func WithTracer(ctx context.Context, tracer ttrace.Tracer) context.Context {
	return context.WithValue(ctx, tracerKey, tracer)
}

func ReqStats(ctx context.Context) metrics.Stats {
	reqStats := ctx.Value(reqStatsKey)
	if t, ok := reqStats.(metrics.Stats); ok {
		return t
	}
	return metrics.NewNoopStats()
}

func WithReqStats(ctx context.Context, stats metrics.Stats) context.Context {
	return context.WithValue(ctx, reqStatsKey, stats)
}

func Span(ctx context.Context) ISpan {
	s := ctx.Value(spanKey)
	if t, ok := s.(*span); ok {
		return t
	}
	return &noopSpan{}
}

func WithSpan(ctx context.Context, name string) (context.Context, *span) {
	ctx, nativeSpan := Tracer(ctx).Start(ctx, name)
	s := &span{Span: nativeSpan, name: name}
	return context.WithValue(ctx, spanKey, s), s
}

type ISpan interface {
	ttrace.Span

	EndWithErr(e *error)
}

type span struct {
	name string
	ttrace.Span
}

func (s *span) EndWithErr(e *error) {
	defer s.Span.End()
	s.SetStatus(codes.Ok, "")

	if e == nil {
		return
	}

	err := *e
	if err == nil {
		return
	}

	if errors.Is(err, io.EOF) {
		return
	}

	s.Span.RecordError(err)
	s.Span.SetStatus(codes.Error, err.Error())
}

func Details(ctx context.Context) *RequestDetails {
	details := ctx.Value(detailsKey)
	if t, ok := details.(*RequestDetails); ok {
		return t
	}
	return nil
}

func WithRequest(ctx context.Context, req *RequestDetails) context.Context {
	return context.WithValue(ctx, detailsKey, req)
}
