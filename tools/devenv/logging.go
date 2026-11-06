package devenv

import (
	"github.com/streamingfast/logging"
	"go.uber.org/zap"
)

var zlog, _ = logging.PackageLogger("devenv", "github.com/streamingfast/substreams/tools/devenv", logging.LoggerDefaultLevel(zap.InfoLevel))
