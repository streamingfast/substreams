package pubsub

import "github.com/streamingfast/logging"

var zlogTest, _ = logging.PackageLogger("pubsub_test", "github.com/streamingfast/substreams/sink/pubsub(test)")

func init() { logging.InstantiateLoggers() }
