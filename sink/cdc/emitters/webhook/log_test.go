package webhook

import "github.com/streamingfast/logging"

var zlog, _ = logging.PackageLogger("sink-cdc-webhook-tests", "github.com/streamingfast/substreams/sink/cdc/emitters/webhook/tests")

func init() {
	logging.InstantiateLoggers()
}
