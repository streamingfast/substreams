package reqctx

import (
	"context"
	"net/http"
	"strconv"

	"github.com/streamingfast/dauth"
)

const HeaderParallelWorkers = "x-substreams-parallel-workers"
const legacyHeaderParallelWorkers = "x-sf-substreams-parallel-jobs"
const HeaderCacheTag = "x-substreams-cache-tag"

// GetEffectiveHeaderValues compares the request headers to the 'trusted headers' sent by the authentication layer.
// It contains some business logic:
//   - prevents overriding the numeric values to lower ones for parallel jobs and stage layer executors
func GetEffectiveHeaderValues(ctx context.Context, headers http.Header, defaultParallelJobs uint64, defaultParallelExecutors uint64) (parallelJobs uint64, parallelExecutors uint64) {

	parallelJobs = defaultParallelJobs
	parallelExecutors = defaultParallelExecutors

	// TrustedHeaders can always override these values
	if trustedHeaders := dauth.FromContext(ctx); trustedHeaders != nil {
		if parallelJobsStr := trustedHeaders.Get(HeaderParallelWorkers); parallelJobsStr != "" {
			if count, err := strconv.ParseUint(parallelJobsStr, 10, 64); err == nil {
				parallelJobs = count
			}
		}

		switch trustedHeaders.SubstreamsPlanTier() {
		case "ENTERPRISE":
			parallelExecutors = 10
		case "PRO":
			parallelExecutors = 8
		case "SCALING":
			parallelExecutors = 5
		default:
			parallelExecutors = 2
		}
	}

	// untrusted headers (from the request) can only reduce the number of parallel workers
	untrustedParallelWorkers := headers.Get(HeaderParallelWorkers)
	if untrustedParallelWorkers == "" {
		untrustedParallelWorkers = headers.Get(legacyHeaderParallelWorkers)
	}
	if untrustedParallelWorkers != "" {
		if count, err := strconv.ParseUint(untrustedParallelWorkers, 10, 64); err == nil {
			if count < parallelJobs {
				parallelJobs = count
			}
		}
	}
	return
}
