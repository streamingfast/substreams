package orchestrator

import (
	"github.com/streamingfast/logging"
	"go.uber.org/zap"
)

var zlog *zap.Logger

func init() {
	zlog, _ = logging.PackageLogger("orchestrator", "github.com/streamingfast/substreams/orchestrator")
}
