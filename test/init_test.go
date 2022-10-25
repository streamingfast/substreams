package integration

import (
	"github.com/streamingfast/logging"
	"github.com/streamingfast/substreams/manifest"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zapcore"
	"testing"
)

var zlog, _ = logging.PackageLogger("pipe.test", "github.com/streamingfast/substreams/test")

func init() {
	logging.InstantiateLoggers(logging.WithDefaultLevel(zapcore.DebugLevel))
}

func processManifest(t *testing.T, manifestPath string) (*pbsubstreams.Package, *manifest.ModuleGraph) {
	t.Helper()

	manifestReader := manifest.NewReader(manifestPath)
	pkg, err := manifestReader.Read()
	require.NoError(t, err)

	moduleGraph, err := manifest.NewModuleGraph(pkg.Modules.Modules)
	require.NoError(t, err)

	return pkg, moduleGraph
}
