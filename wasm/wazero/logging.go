package wazero

import (
	"github.com/streamingfast/logging"
)

var zlog, tracer = logging.PackageLogger("wazero-runtime", "github.com/streamingfast/substreams/wasm/wazero")
