package metrics

import (
	"context"
	"errors"

	"connectrpc.com/connect"
	"github.com/streamingfast/dgrpc"
	"google.golang.org/grpc/codes"
)

// gRPC and ConnectRPC codes are strictly aligned, so we can use the same map for both, one difference
// is that they don't define OK as a code, but not a problem here as we have it for gRPC code.
var grpcCodeToLabel = map[codes.Code]string{
	codes.OK:                 "ok",
	codes.Canceled:           "cancelled",
	codes.Unknown:            "unknown",
	codes.InvalidArgument:    "invalid_argument",
	codes.DeadlineExceeded:   "deadline_exceeded",
	codes.NotFound:           "not_found",
	codes.AlreadyExists:      "already_exists",
	codes.PermissionDenied:   "permission_denied",
	codes.ResourceExhausted:  "resource_exhausted",
	codes.FailedPrecondition: "failed_precondition",
	codes.Aborted:            "aborted",
	codes.OutOfRange:         "out_of_range",
	codes.Unimplemented:      "unimplemented",
	codes.Internal:           "internal",
	codes.Unavailable:        "unavailable",
	codes.DataLoss:           "data_loss",
	codes.Unauthenticated:    "unauthenticated",
}

// CodeToRejectedReason accepts a ConnectRPC code () or a gRPC code (google.golang.org/grpc/codes.Code)
// and returns the corresponding label string we use in metrics for rejected request.
func codeToRejectedReason(code uint32) string {
	label, found := grpcCodeToLabel[codes.Code(code)]
	if !found {
		return "unknown"
	}

	return label
}

// IsRejectedRequestError returns true if the error is a rejected request error
// and should be logged as such in metrics.
func IsRejectedRequestError(err error) (metricsReason string, countAsRejected bool) {
	if err == nil {
		return "", false
	}
	if errors.Is(err, context.Canceled) {
		return "cancelled", false
	}

	errorCode := errorToRejectedReason(err)
	if errorCode == "ok" || errorCode == "cancelled" {
		return "", false
	}

	return errorCode, true
}

// ErrorToRejectedReason accepts an error, extracts the code from it (accepting gRPC and ConnectRPC error)
// and returns the corresponding label string we use in metrics for reject request.
func errorToRejectedReason(err error) string {
	if grpcErr := dgrpc.AsGRPCError(err); grpcErr != nil {
		return codeToRejectedReason(uint32(grpcErr.Code()))
	}

	if connectErr := (*connect.Error)(nil); errors.As(err, &connectErr) {
		return codeToRejectedReason(uint32(connectErr.Code()))
	}

	return "unknown"
}
