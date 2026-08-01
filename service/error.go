package service

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	bsstream "github.com/streamingfast/bstream/stream"
	"github.com/streamingfast/dgrpc"
	"github.com/streamingfast/dsession"
	"github.com/streamingfast/substreams/pipeline"
	"github.com/streamingfast/substreams/storage/store"
	"github.com/streamingfast/substreams/wasm"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// toConnectError turns an `err` into a connect error if it's non-nil, in the `nil` case,
// `nil` is returned right away.
//
// If the `err` has in its chain of error either `context.Canceled`, `context.DeadlineExceeded`
// or `stream.ErrInvalidArg`, error is turned into a proper connect error respectively of code
// `Canceled`, `DeadlineExceeded` or `InvalidArgument`.
//
// If the `err` has in its chain any error constructed through `connect.NewError` (and its variants), then
// we return the first found error of such type directly, because it's already a connect error.
//
// If the `err` has in its chain any error constructed through `grpc` or `status`, it will be converted to connect equivalent.
//
// Otherwise, the error is assumed to be an internal error and turned backed into a proper
// `connect.NewError(connect.CodeInternal, err)`.
func toConnectError(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}

	if errors.Is(err, context.Canceled) {
		if contextCause := context.Cause(ctx); contextCause != nil {
			err = contextCause // unwrap errors in canceled contexts
			if errors.Is(err, context.Canceled) {
				return connect.NewError(connect.CodeCanceled, err)
			}
		} else {
			return connect.NewError(connect.CodeCanceled, err)
		}
	}

	if err, ok := dsession.ToConnectError(err); ok {
		return err
	}
	// special case for context canceled when shutting down. The error is wrapped by the
	// time it reaches here (ex: `error during init_stores_and_backprocess: ... scheduler
	// run: %w`), so it must be matched with `errors.Is` and not a pointer comparison.
	if errors.Is(err, errShuttingDown) {
		return connect.NewError(connect.CodeUnavailable, err)
	}

	// GRPC to connect error
	if grpcError := dgrpc.AsGRPCError(err); grpcError != nil {
		switch grpcError.Code() {
		case codes.Canceled:
			return connect.NewError(connect.CodeCanceled, errors.New(grpcError.Message()))
		case codes.Unavailable:
			return connect.NewError(connect.CodeUnavailable, errors.New(grpcError.Message()))
		case codes.InvalidArgument:
			return connect.NewError(connect.CodeInvalidArgument, errors.New(grpcError.Message()))
		case codes.DeadlineExceeded:
			return connect.NewError(connect.CodeDeadlineExceeded, errors.New(grpcError.Message()))
		case codes.ResourceExhausted:
			return connect.NewError(connect.CodeResourceExhausted, errors.New(grpcError.Message()))
		case codes.FailedPrecondition:
			return connect.NewError(connect.CodeFailedPrecondition, errors.New(grpcError.Message()))
		case codes.Aborted:
			return connect.NewError(connect.CodeAborted, errors.New(grpcError.Message()))
		case codes.OutOfRange:
			return connect.NewError(connect.CodeOutOfRange, errors.New(grpcError.Message()))
		case codes.Unimplemented:
			return connect.NewError(connect.CodeUnimplemented, errors.New(grpcError.Message()))
		case codes.Internal:
			return connect.NewError(connect.CodeInternal, errors.New(grpcError.Message()))
		case codes.Unknown:
			return connect.NewError(connect.CodeUnknown, errors.New(grpcError.Message()))
		case codes.NotFound:
			return connect.NewError(connect.CodeNotFound, errors.New(grpcError.Message()))
		case codes.AlreadyExists:
			return connect.NewError(connect.CodeAlreadyExists, errors.New(grpcError.Message()))
		case codes.PermissionDenied:
			return connect.NewError(connect.CodePermissionDenied, errors.New(grpcError.Message()))
		case codes.Unauthenticated:
			return connect.NewError(connect.CodeUnauthenticated, errors.New(grpcError.Message()))
		case codes.DataLoss:
			return connect.NewError(connect.CodeDataLoss, errors.New(grpcError.Message()))
		default:
			return connect.NewError(connect.CodeUnknown, errors.New(grpcError.Message()))
		}
	}

	// special case for "QuickSave" on shutdown
	if errors.Is(err, pipeline.ErrShuttingDown) {
		return connect.NewError(connect.CodeUnavailable, err)
	}

	// context deadline exceeded
	if errors.Is(err, context.DeadlineExceeded) {
		return connect.NewError(connect.CodeDeadlineExceeded, err)
	}

	if errors.Is(err, wasm.ErrWasmDeterministicExec) || errors.Is(err, store.ErrStoreAboveMaxSize) {
		return connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("%w (new deterministic error)", err))
	}

	var errInvalidArg *bsstream.ErrInvalidArg
	if errors.As(err, &errInvalidArg) {
		return connect.NewError(connect.CodeInvalidArgument, errInvalidArg)
	}

	connectError := new(connect.Error)
	if errors.As(err, &connectError) {
		return connectError
	}

	// Do we want to print the full cause as coming from Golang? Would we like to maybe trim off "operational"
	// data?
	return connect.NewError(connect.CodeInternal, err)
}

func toGrpcTier1Error(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}

	if errors.Is(err, context.Canceled) {
		if contextCause := context.Cause(ctx); contextCause != nil {
			err = contextCause // unwrap errors in canceled contexts
			if errors.Is(err, context.Canceled) {
				// Return a gRPC status error (like every other branch below) so the caller's
				// status.Code() resolves to codes.Canceled and logs at Debug. A connect error
				// here resolves to codes.Unknown, wrongly logging a client disconnect as WARN
				// (and the gRPC middleware then reports "code Unknown" at ERROR).
				return status.Error(codes.Canceled, err.Error())
			}
		} else {
			return status.Error(codes.Canceled, err.Error())
		}
	}

	// GRPC error directly
	if grpcError := dgrpc.AsGRPCError(err); grpcError != nil {
		return err
	}

	// convert connectWeb error
	connectError := new(connect.Error)
	if errors.As(err, &connectError) {
		code := connectError.Code()
		switch code {
		case connect.CodeCanceled:
			return status.Error(codes.Canceled, connectError.Message())
		case connect.CodeUnavailable:
			return status.Error(codes.Unavailable, connectError.Message())
		case connect.CodeInvalidArgument:
			return status.Error(codes.InvalidArgument, connectError.Message())
		case connect.CodeDeadlineExceeded:
			return status.Error(codes.DeadlineExceeded, connectError.Message())
		case connect.CodeResourceExhausted:
			return status.Error(codes.ResourceExhausted, connectError.Message())
		case connect.CodeFailedPrecondition:
			return status.Error(codes.FailedPrecondition, connectError.Message())
		case connect.CodeAborted:
			return status.Error(codes.Aborted, connectError.Message())
		case connect.CodeOutOfRange:
			return status.Error(codes.OutOfRange, connectError.Message())
		case connect.CodeUnimplemented:
			return status.Error(codes.Unimplemented, connectError.Message())
		case connect.CodeInternal:
			return status.Error(codes.Internal, connectError.Message())
		case connect.CodeUnknown:
			return status.Error(codes.Unknown, connectError.Message())
		case connect.CodeNotFound:
			return status.Error(codes.NotFound, connectError.Message())
		case connect.CodeAlreadyExists:
			return status.Error(codes.AlreadyExists, connectError.Message())
		case connect.CodePermissionDenied:
			return status.Error(codes.PermissionDenied, connectError.Message())
		case connect.CodeUnauthenticated:
			return status.Error(codes.Unauthenticated, connectError.Message())
		case connect.CodeDataLoss:
			return status.Error(codes.DataLoss, connectError.Message())
		default:
			return status.Error(codes.Unknown, connectError.Message())
		}
	}

	// convert dsession error
	if errors.Is(err, dsession.ErrUnavailable) {
		return status.Error(codes.Unavailable, err.Error())
	}
	if errors.Is(err, dsession.ErrPermissionDenied) {
		return status.Error(codes.PermissionDenied, err.Error())
	}
	if errors.Is(err, dsession.ErrQuotaExceeded) {
		return status.Error(codes.ResourceExhausted, err.Error())
	}
	if errors.Is(err, dsession.ErrConcurrentStreamLimitExceeded) {
		return status.Error(codes.ResourceExhausted, err.Error())
	}

	// special case for context canceled when shutting down
	if errors.Is(err, errShuttingDown) {
		return status.Error(codes.Unavailable, err.Error())
	}

	// special case for "QuickSave" on shutdown
	if errors.Is(err, pipeline.ErrShuttingDown) {
		return status.Error(codes.Unavailable, err.Error())
	}

	// context deadline exceeded
	if errors.Is(err, context.DeadlineExceeded) {
		return status.Error(codes.DeadlineExceeded, err.Error())
	}

	if errors.Is(err, wasm.ErrWasmDeterministicExec) || errors.Is(err, store.ErrStoreAboveMaxSize) {
		return status.Errorf(codes.InvalidArgument, "%s (new deterministic error)", err)
	}

	var errInvalidArg *bsstream.ErrInvalidArg
	if errors.As(err, &errInvalidArg) {
		return status.Error(codes.InvalidArgument, errInvalidArg.Error())
	}

	// Do we want to print the full cause as coming from Golang? Would we like to maybe trim off "operational"
	// data?
	return status.Error(codes.Internal, err.Error())
}
