package errors

import (
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type GRPCError interface {
	Cause() error
	RpcErr() error
}

type BasicErr struct {
	inner   error
	grpcErr error
}

func NewBasicErr(grpcErr, inner error) BasicErr { return BasicErr{grpcErr: grpcErr, inner: inner} }
func (e BasicErr) Cause() error                 { return e.inner }
func (e BasicErr) RpcErr() error                { return e.grpcErr }

type ErrSendBlock struct {
	Inner error
}

func NewErrSendBlock(inner error) ErrSendBlock { return ErrSendBlock{Inner: inner} }
func (e ErrSendBlock) Cause() error            { return e.Inner }
func (e ErrSendBlock) RpcErr() error           { return status.Error(codes.Unavailable, e.Inner.Error()) }
func (e ErrSendBlock) Error() string           { return e.Inner.Error() }

type ErrDeadlineExceeded struct {
	inner error
}

func NewErrDeadlineExceeded(inner error) ErrDeadlineExceeded {
	return ErrDeadlineExceeded{inner: inner}
}
func (e ErrDeadlineExceeded) Cause() error { return e.inner }
func (e ErrDeadlineExceeded) RpcErr() error {
	return status.Error(codes.DeadlineExceeded, "source deadline exceeded")
}

type ErrContextCanceled struct {
	inner error
}

func NewErrContextCanceled(inner error) ErrContextCanceled { return ErrContextCanceled{inner: inner} }
func (e ErrContextCanceled) Cause() error                  { return e.inner }
func (e ErrContextCanceled) RpcErr() error                 { return status.Error(codes.Canceled, "source canceled") }
