package sink

import "github.com/streamingfast/logging"

var zlog, _ = logging.PackageLogger("sink-tests", "github.com/streamingfast/substreams-sink/tests")

func init() {
	logging.InstantiateLoggers()
}
