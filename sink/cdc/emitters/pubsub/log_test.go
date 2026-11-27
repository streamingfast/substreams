package pubsub

import "github.com/streamingfast/logging"

var zlog, _ = logging.PackageLogger("sink-cdc-pubsub-tests", "github.com/streamingfast/substreams/sink/cdc/emitters/pubsub/tests")

func init() {
	logging.InstantiateLoggers()
}
