package writer

import "github.com/streamingfast/logging"

var zlog, _ = logging.PackageLogger("writer", "github.com/streamingfast/substreams-graph-load/bundler/writer_test")

func init() {
	logging.InstantiateLoggers()
}
