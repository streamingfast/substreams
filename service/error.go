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
	// special case for context canceled when shutting down
	if err == errShuttingDown {
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
			return connect.NewError(connect.CodeDeadlineExceeded, err)
		case codes.ResourceExhausted:
			return connect.NewError(connect.CodeResourceExhausted, errors.New(grpcError.Message()))
		case codes.Unknown:
			return connect.NewError(connect.CodeUnknown, errors.New(grpcError.Message()))
		}
		return grpcError.Err()
	}

	// special case for "QuickSave" on shutdown
	if err == pipeline.ErrShuttingDown {
		return connect.NewError(connect.CodeUnavailable, err)
	}

	// context deadline exceeded
	if errors.Is(err, context.DeadlineExceeded) {
		return connect.NewError(connect.CodeDeadlineExceeded, err)
	}

	if errors.Is(err, wasm.ErrWasmDeterministicExec) || errors.Is(err, store.ErrStoreAboveMaxSize) {
		return connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("%w (deterministic error)", err))
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
				return connect.NewError(connect.CodeCanceled, err)
			}
		} else {
			return connect.NewError(connect.CodeCanceled, err)
		}
	}

	grpcError := func(err error) (error, bool) {
		// Handle session pool errors
		if errors.Is(err, dsession.ErrUnavailable) {
			return status.Error(codes.Unavailable, err.Error()), true
		}
		if errors.Is(err, dsession.ErrPermissionDenied) {
			return status.Error(codes.PermissionDenied, err.Error()), true
		}
		if errors.Is(err, dsession.ErrQuotaExceeded) {
			return status.Error(codes.ResourceExhausted, err.Error()), true
		}
		if errors.Is(err, dsession.ErrConcurrentStreamLimitExceeded) {
			return status.Error(codes.ResourceExhausted, err.Error()), true
		}
		return err, false
	}

	if err, ok := grpcError(err); ok {
		return err
	}
	// special case for context canceled when shutting down
	if errors.Is(err, errShuttingDown) {
		return status.Error(codes.Unavailable, err.Error())
	}

	// GRPC to connect error
	if grpcError := dgrpc.AsGRPCError(err); grpcError != nil {
		return grpcError.Err()
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
		return status.Errorf(codes.InvalidArgument, "%s (deterministic error)", err)
	}

	var errInvalidArg *bsstream.ErrInvalidArg
	if errors.As(err, &errInvalidArg) {
		return status.Error(codes.InvalidArgument, errInvalidArg.Error())
	}

	// Do we want to print the full cause as coming from Golang? Would we like to maybe trim off "operational"
	// data?
	return status.Error(codes.Internal, err.Error())
}
