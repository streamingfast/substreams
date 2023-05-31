package wasmtime

import (
	"github.com/streamingfast/logging"
)

var zlog, tracer = logging.PackageLogger("wasmtime-runtime", "github.com/streamingfast/substreams/wasm/wasmtime")
