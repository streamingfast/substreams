package main

import (
	"github.com/streamingfast/logging"
)

var zlog, tracer = logging.RootLogger("substreams", "github.com/streamingfast/substreams/cmd/substreams")

// loggers are instantiated in 'setup()' func
