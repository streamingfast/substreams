package metrics

import (
	"github.com/streamingfast/logging"
)

var zlogTest, _ = logging.PackageLogger("metrics.test", "github.com/streamingfast/substreams/metrics.test")

func init() {
	logging.InstantiateLoggers()
}
