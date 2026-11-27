package file

import "github.com/streamingfast/logging"

var zlog, _ = logging.PackageLogger("sink-cdc-file-tests", "github.com/streamingfast/substreams/sink/cdc/emitters/file/tests")

func init() {
	logging.InstantiateLoggers()
}
