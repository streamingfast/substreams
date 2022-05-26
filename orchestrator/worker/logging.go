package worker

import (
	"github.com/streamingfast/logging"
)

var zlog, tracer = logging.PackageLogger("worker", "github.com/streamingfast/substreams/orchestrator/worker")
