package reqctx

import (
	"context"
	"errors"
	"io"

	"go.opentelemetry.io/otel/attribute"

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
var moduleExecutionTracingConfigKey = contextKeyType(5)

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

func ReqStats(ctx context.Context) *metrics.Stats {
	return ctx.Value(reqStatsKey).(*metrics.Stats)
}

func WithReqStats(ctx context.Context, stats *metrics.Stats) context.Context {
	return context.WithValue(ctx, reqStatsKey, stats)
}

func Span(ctx context.Context) ISpan {
	s := ctx.Value(spanKey)
	if t, ok := s.(*span); ok {
		return t
	}
	return &NoopSpan{}
}

func WithModuleExecutionSpan(ctx context.Context, name string) (context.Context, ISpan) {
	if !ModuleExecutionTracing(ctx) {
		return ctx, &NoopSpan{}
	}
	ctx, nativeSpan := Tracer(ctx).Start(ctx, name)
	s := &span{Span: nativeSpan, name: name}
	return context.WithValue(ctx, spanKey, s), s
}

func WithSpan(ctx context.Context, name string) (context.Context, ISpan) {
	ctx, nativeSpan := Tracer(ctx).Start(ctx, name)
	s := &span{Span: nativeSpan, name: name}
	return context.WithValue(ctx, spanKey, s), s
}

type ISpan interface {
	// End completes the Span. The Span is considered complete and ready to be
	// delivered through the rest of the telemetry pipeline after this method
	// is called. Therefore, updates to the Span are not allowed after this
	// method has been called.
	End(options ...ttrace.SpanEndOption)

	// AddEvent adds an event with the provided name and options.
	AddEvent(name string, options ...ttrace.EventOption)

	// IsRecording returns the recording state of the Span. It will return
	// true if the Span is active and events can be recorded.
	IsRecording() bool

	// RecordError will record err as an exception span event for this span. An
	// additional call to SetStatus is required if the Status of the Span should
	// be set to Error, as this method does not change the Span status. If this
	// span is not being recorded or err is nil then this method does nothing.
	RecordError(err error, options ...ttrace.EventOption)

	// SpanContext returns the SpanContext of the Span. The returned SpanContext
	// is usable even after the End method has been called for the Span.
	SpanContext() ttrace.SpanContext

	// SetStatus sets the status of the Span in the form of a code and a
	// description, provided the status hasn't already been set to a higher
	// value before (OK > Error > Unset). The description is only included in a
	// status when the code is for an error.
	SetStatus(code codes.Code, description string)

	// SetName sets the Span name.
	SetName(name string)

	// SetAttributes sets kv as attributes of the Span. If a key from kv
	// already exists for an attribute of the Span it will be overwritten with
	// the value contained in kv.
	SetAttributes(kv ...attribute.KeyValue)

	// TracerProvider returns a TracerProvider that can be used to generate
	// additional Spans on the same telemetry pipeline as the current Span.
	TracerProvider() ttrace.TracerProvider

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

func ModuleExecutionTracing(ctx context.Context) bool {
	tracer := ctx.Value(moduleExecutionTracingConfigKey)
	if t, ok := tracer.(bool); ok {
		return t
	}
	return false
}

func WithModuleExecutionTracing(ctx context.Context) context.Context {
	return context.WithValue(ctx, moduleExecutionTracingConfigKey, true)
}
