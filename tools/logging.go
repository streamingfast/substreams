package tools

import (
	"github.com/streamingfast/logging"
	"go.uber.org/zap"
)

var zlog *zap.Logger

func init() {
	zlog, _ = logging.ApplicationLogger("substreams", "github.com/streamingfast/substreams/tools",
		logging.WithSwitcherServerAutoStart(),
	)
}
