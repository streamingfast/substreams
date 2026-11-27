package cdc

import "github.com/streamingfast/logging"

var zlog, _ = logging.PackageLogger("sink-cdc-tests", "github.com/streamingfast/substreams/sink/cdc/tests")

func init() {
	logging.InstantiateLoggers()
}
