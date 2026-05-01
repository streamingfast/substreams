package main

import (
	"github.com/streamingfast/cli"
	"github.com/streamingfast/logging"
	"go.uber.org/zap"
)

var zlog, tracer = logging.RootLogger("sink-sql", "github.com/streamingfast/substreams/cmd/substreams-sink-sql")

func init() {
	cli.SetLogger(zlog, tracer)

	logging.InstantiateLoggers(logging.WithDefaultLevel(zap.InfoLevel))
}
