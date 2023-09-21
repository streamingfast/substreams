package main

import (
	"github.com/streamingfast/logging"
	"go.uber.org/zap/zapcore"
)

var zlog, tracer = logging.RootLogger("substreams", "github.com/streamingfast/substreams/cmd/substreams")

func init() {
	logging.InstantiateLoggers(logging.WithLogLevelSwitcherServerAutoStart(), logging.WithDefaultLevel(zapcore.WarnLevel))
}
