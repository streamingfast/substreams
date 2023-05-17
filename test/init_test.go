package integration

import (
	"github.com/streamingfast/logging"
)

var zlog, _ = logging.PackageLogger("pipe.test", "github.com/streamingfast/substreams/test")

func init() {
	//logging.InstantiateLoggers(logging.WithDefaultLevel(zapcore.InfoLevel))
}
