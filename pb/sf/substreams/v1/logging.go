package pbsubstreams

import (
	"github.com/streamingfast/logging"
	"go.uber.org/zap"
)

var zlog *zap.Logger

func init() {
	zlog, _ = logging.PackageLogger("substreams_v1", "github.com/streamingfast/substreams/pb/sf/substreams/v1/")
}
