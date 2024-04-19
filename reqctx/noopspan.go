package reqctx

import (
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	ttrace "go.opentelemetry.io/otel/trace"
)

// NoopSpan is an implementation of span that preforms no operations.
type NoopSpan struct{}

func (n *NoopSpan) EndWithErr(e *error) {}

// SpanContext returns an empty span context.
func (n *NoopSpan) SpanContext() ttrace.SpanContext { return ttrace.SpanContext{} }

// IsRecording always returns false.
func (n *NoopSpan) IsRecording() bool { return false }

// SetStatus does nothing.
func (n *NoopSpan) SetStatus(codes.Code, string) {}

// SetError does nothing.
func (n *NoopSpan) SetError(bool) {}

// SetAttributes does nothing.
func (n *NoopSpan) SetAttributes(...attribute.KeyValue) {}

// End does nothing.
func (n *NoopSpan) End(...ttrace.SpanEndOption) {}

// RecordError does nothing.
func (n *NoopSpan) RecordError(error, ...ttrace.EventOption) {}

// AddEvent does nothing.
func (n *NoopSpan) AddEvent(string, ...ttrace.EventOption) {}

// SetName does nothing.
func (n *NoopSpan) SetName(string) {}

// TracerProvider returns a no-op TracerProvider.
func (n *NoopSpan) TracerProvider() ttrace.TracerProvider { return ttrace.NewNoopTracerProvider() }
