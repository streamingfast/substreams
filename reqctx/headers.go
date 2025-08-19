package reqctx

import (
	"context"
	"net/http"
	"strconv"

	"github.com/streamingfast/dauth"
)

const HeaderParallelJobs = "X-Sf-Substreams-Parallel-Jobs"
const HeaderCacheTag = "X-Sf-Substreams-Cache-Tag"
const HeaderParallelExecutor = "X-Sf-Substreams-Stage-Layer-Parallel-Executor-Max-Count"

// GetEffectiveHeaderValues compares the request headers to the 'trusted headers' sent by the authentication layer.
// It contains some business logic:
//   - prevents overriding the numeric values to lower ones for parallel jobs and stage layer executors
func GetEffectiveHeaderValues(ctx context.Context, headers http.Header, defaultParallelJobs uint64, defaultParallelExecutors uint64) (parallelJobs uint64, parallelExecutors uint64) {

	parallelJobs = defaultParallelJobs
	parallelExecutors = defaultParallelExecutors

	// TrustedHeaders can always override these values
	if trustedHeaders := dauth.FromContext(ctx); trustedHeaders != nil {
		if parallelJobsStr := trustedHeaders.Get(HeaderParallelJobs); parallelJobsStr != "" {
			if count, err := strconv.ParseUint(parallelJobsStr, 10, 64); err == nil {
				parallelJobs = count
			}
		}
		if parallelExecutorsStr := trustedHeaders.Get(HeaderParallelExecutor); parallelExecutorsStr != "" {
			if count, err := strconv.ParseUint(parallelExecutorsStr, 10, 64); err == nil {
				parallelJobs = count
			}
		}
	}

	// Normal headers can only lower those values
	if parallelJobsStr := headers.Get(HeaderParallelJobs); parallelJobsStr != "" {
		if count, err := strconv.ParseUint(parallelJobsStr, 10, 64); err == nil {
			if count < parallelJobs {
				count = parallelJobs
			}
			parallelJobs = count
		}
	}
	if parallelExecutorsStr := headers.Get(HeaderParallelExecutor); parallelExecutorsStr != "" {
		if count, err := strconv.ParseUint(parallelExecutorsStr, 10, 64); err == nil {
			if count < parallelExecutors {
				count = parallelExecutors
			}
			parallelExecutors = count
		}
	}

	return
}
