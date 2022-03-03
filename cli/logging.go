package cli

import (
	"github.com/streamingfast/logging"
	zapbox "github.com/streamingfast/substreams/zap-box"
	"go.uber.org/zap"
)

var zlog *zap.Logger

func init() {
	encoder := zapbox.NewEncoder(1)
	zlog, _ = logging.ApplicationLogger("substreams", "github.com/streamingfast/substreams/cmd/substreams",
		logging.WithSwitcherServerAutoStart(),
		logging.WithEncoder(encoder),
	)
}
