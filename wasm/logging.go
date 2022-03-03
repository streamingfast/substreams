package wasm

import (
	"github.com/streamingfast/logging"
	"go.uber.org/zap"
)

var zlog *zap.Logger

func init() {
	zlog, _ = logging.PackageLogger("wasm-runtime", "github.com/streamingfast/substreams/wasm")
}
