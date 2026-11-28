package rows

import "github.com/streamingfast/logging"

var zlog, _ = logging.PackageLogger("sink-rows-tests", "github.com/streamingfast/substreams/sink/rows/tests")

func init() {
	logging.InstantiateLoggers()
}
