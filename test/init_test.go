package pipeline

import (
	"github.com/streamingfast/logging"
	"go.uber.org/zap/zapcore"
)

func init() {
	logging.InstantiateLoggers(logging.WithDefaultLevel(zapcore.WarnLevel))
}
