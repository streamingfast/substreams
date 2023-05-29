package bigdecimal

import "github.com/streamingfast/logging"

// Only used in `big_decimal` which is gated with a special in-code variable to turn it on.
// should not be used anywhere else.
var zlog, tracer = logging.PackageLogger("bigdecimal", "github.com/streamingfast/substreams/bigdecimal")
