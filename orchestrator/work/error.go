package work

import "errors"

var ErrorResourceExhausted = errors.New("resource exhausted")
var ErrorResourceExhaustedRampUp = errors.New("resource exhausted during ramp up")

type RetryableErr struct {
	cause error
}

func NewRetryableErr(cause error) *RetryableErr {
	return &RetryableErr{
		cause: cause,
	}
}

func (r *RetryableErr) Error() string {
	return r.cause.Error()
}

func (r *RetryableErr) Unwrap() error {
	return r.cause
}
