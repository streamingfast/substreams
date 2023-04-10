package main

import (
	"github.com/streamingfast/logging"
)

var zlog, _ = logging.RootLogger("substreams", "github.com/streamingfast/substreams/cmd/substreams")

func init() {
	logging.InstantiateLoggers(logging.WithSwitcherServerAutoStart())
}
