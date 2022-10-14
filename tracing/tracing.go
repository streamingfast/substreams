package tracing

import (
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

type EndSpanOption interface {
	applyToSpan(span trace.Span)
}

type withEndError struct {
	err error
}

func (e withEndError) applyToSpan(span trace.Span) {
	span.RecordError(e.err)
	span.SetStatus(codes.Error, e.err.Error())
}

// WithEndErr is an option that can be apply to `EndSpan`, if the receiced
// `err` is non-nil, the method `Span.RecordError(err)` and `Span.SetStatus(codes.Error, ...)`
// will be called on the span when it ends.
//
// Otherwise, if the `err` is nil, a no-op option is returned.
func WithEndErr(err error) EndSpanOption {
	if err == nil {
		return noOpEndSpanOptionSingleton
	}

	return withEndError{err}
}

var noOpEndSpanOptionSingleton *noOpEndSpanOption = nil

type noOpEndSpanOption struct{}

func (o *noOpEndSpanOption) applyToSpan(span trace.Span) {
}

func EndSpan(span trace.Span, options ...EndSpanOption) {
	span.SetStatus(codes.Ok, "")
	for _, opt := range options {
		opt.applyToSpan(span)
	}
}
