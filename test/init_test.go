package integration

import (
	"github.com/streamingfast/logging"
	"go.uber.org/zap/zapcore"
)

var zlog, _ = logging.PackageLogger("pipe.test", "github.com/streamingfast/substreams/test")

func init() {
	// To tweak in tests, add DEBUG=true in your ENV VARS dude.
	logging.InstantiateLoggers(logging.WithDefaultLevel(zapcore.DebugLevel))
}
