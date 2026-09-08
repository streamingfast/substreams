package stage

import (
	"github.com/streamingfast/logging"
)

var zlogTest, _ = logging.PackageLogger("stage.test", "github.com/streamingfast/substreams/orchestrator/stage.test")

func init() {
	logging.InstantiateLoggers()
}
