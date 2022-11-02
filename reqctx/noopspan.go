package reqctx

import (
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	ttrace "go.opentelemetry.io/otel/trace"
)

// noopSpan is an implementation of Span that preforms no operations.
type noopSpan struct{}

func (n *noopSpan) EndWithErr(e *error) {
	panic("implement me")
}

// SpanContext returns an empty span context.
func (n *noopSpan) SpanContext() ttrace.SpanContext { return ttrace.SpanContext{} }

// IsRecording always returns false.
func (n *noopSpan) IsRecording() bool { return false }

// SetStatus does nothing.
func (n *noopSpan) SetStatus(codes.Code, string) {}

// SetError does nothing.
func (n *noopSpan) SetError(bool) {}

// SetAttributes does nothing.
func (n *noopSpan) SetAttributes(...attribute.KeyValue) {}

// End does nothing.
func (n *noopSpan) End(...ttrace.SpanEndOption) {}

// RecordError does nothing.
func (n *noopSpan) RecordError(error, ...ttrace.EventOption) {}

// AddEvent does nothing.
func (n *noopSpan) AddEvent(string, ...ttrace.EventOption) {}

// SetName does nothing.
func (n *noopSpan) SetName(string) {}

// TracerProvider returns a no-op TracerProvider.
func (n *noopSpan) TracerProvider() ttrace.TracerProvider { return ttrace.NewNoopTracerProvider() }
