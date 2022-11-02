package integration

import (
	"github.com/streamingfast/logging"
	"go.uber.org/zap/zapcore"
)

var zlog, _ = logging.PackageLogger("pipe.test", "github.com/streamingfast/substreams/test")

func init() {
	logging.InstantiateLoggers(logging.WithDefaultLevel(zapcore.DebugLevel)) // JULIEN PLEASE DON'T TOUCH (╯°□°)╯︵ ┻━┻
}
