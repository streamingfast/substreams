package reqctx

import (
	"context"
	"net/http"
	"strconv"

	"github.com/streamingfast/dauth"
	"go.uber.org/zap/zapcore"
)

const HeaderParallelWorkers = "x-substreams-parallel-workers"
const legacyHeaderParallelWorkers = "x-sf-substreams-parallel-jobs"
const HeaderCacheTag = "x-substreams-cache-tag"

// Possible values of EffectiveParallelism.WorkersSource, describing what determined
// the effective worker count of a request.
const (
	WorkersSourceDefault       = "default"
	WorkersSourceTrustedHeader = "trusted_header"
	WorkersSourceClientHeader  = "client_header"
)

// EffectiveParallelism is the outcome of the parallelism negotiation between the
// server defaults, the trusted headers set by the authentication layer and the
// headers sent by the client. It keeps each input around (instead of only the final
// value) so that a request log line is enough to explain why a client asking for N
// workers ended up with a different number.
type EffectiveParallelism struct {
	// GrantedWorkers is the worker count the authentication layer allows for this
	// request, falling back to the server default when the trusted header is absent.
	GrantedWorkers uint64

	// RequestedWorkers is the worker count the client asked for through the untrusted
	// header, 0 when the client did not ask for anything.
	RequestedWorkers uint64

	// Workers is the effective worker count: min(GrantedWorkers, RequestedWorkers) when
	// the client asked for less than what it was granted, GrantedWorkers otherwise.
	Workers uint64

	// StageLayerExecutors is the number of modules that can be executed in parallel
	// within a single stage layer, derived from the plan tier.
	StageLayerExecutors uint64

	// PlanTier is the substreams plan tier reported by the authentication layer, empty
	// when the request is unauthenticated.
	PlanTier string

	// WorkersSource tells which of the three inputs determined Workers, one of
	// WorkersSourceDefault, WorkersSourceTrustedHeader or WorkersSourceClientHeader.
	WorkersSource string
}

func (p EffectiveParallelism) MarshalLogObject(encoder zapcore.ObjectEncoder) error {
	encoder.AddUint64("requested_workers", p.RequestedWorkers)
	encoder.AddUint64("granted_workers", p.GrantedWorkers)
	encoder.AddUint64("workers", p.Workers)
	encoder.AddString("workers_source", p.WorkersSource)
	encoder.AddString("plan_tier", p.PlanTier)
	encoder.AddUint64("stage_layer_executors", p.StageLayerExecutors)
	return nil
}

// GetEffectiveHeaderValues compares the request headers to the 'trusted headers' sent by the authentication layer.
// It contains some business logic:
//   - prevents overriding the numeric values to lower ones for parallel jobs and stage layer executors
func GetEffectiveHeaderValues(ctx context.Context, headers http.Header, defaultParallelJobs uint64, defaultParallelExecutors uint64) EffectiveParallelism {
	out := EffectiveParallelism{
		GrantedWorkers:      defaultParallelJobs,
		Workers:             defaultParallelJobs,
		StageLayerExecutors: defaultParallelExecutors,
		WorkersSource:       WorkersSourceDefault,
	}

	// TrustedHeaders can always override these values
	if trustedHeaders := dauth.FromContext(ctx); trustedHeaders != nil {
		if parallelJobsStr := trustedHeaders.Get(HeaderParallelWorkers); parallelJobsStr != "" {
			if count, err := strconv.ParseUint(parallelJobsStr, 10, 64); err == nil {
				out.GrantedWorkers = count
				out.Workers = count
				out.WorkersSource = WorkersSourceTrustedHeader
			}
		}

		out.PlanTier = trustedHeaders.SubstreamsPlanTier()
		switch out.PlanTier {
		case "ENTERPRISE":
			out.StageLayerExecutors = 10
		case "PRO":
			out.StageLayerExecutors = 8
		case "SCALING":
			out.StageLayerExecutors = 5
		default:
			out.StageLayerExecutors = 2
		}
	}

	// untrusted headers (from the request) can only reduce the number of parallel workers
	untrustedParallelWorkers := headers.Get(HeaderParallelWorkers)
	if untrustedParallelWorkers == "" {
		untrustedParallelWorkers = headers.Get(legacyHeaderParallelWorkers)
	}
	if untrustedParallelWorkers != "" {
		if count, err := strconv.ParseUint(untrustedParallelWorkers, 10, 64); err == nil {
			out.RequestedWorkers = count
			if count < out.Workers {
				out.Workers = count
				out.WorkersSource = WorkersSourceClientHeader
			}
		}
	}

	return out
}
