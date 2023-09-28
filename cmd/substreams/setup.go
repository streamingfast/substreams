package main

import (
	"net/http"
	_ "net/http/pprof"

	"github.com/spf13/cobra"
	"github.com/streamingfast/logging"
	"github.com/streamingfast/substreams/manifest"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func setup(cmd *cobra.Command, loglevel zapcore.Level) {
	setupProfiler()
	manifest.IPFSURL = mustGetString(cmd, "ipfs-url")
	logging.InstantiateLoggers(logging.WithLogLevelSwitcherServerAutoStart(), logging.WithDefaultLevel(loglevel))
}

var (
	pprofListenAddr = "localhost:6060"
)

func setupProfiler() {
	go func() {
		err := http.ListenAndServe(pprofListenAddr, nil)
		if err != nil {
			zlog.Debug("unable to start profiling server", zap.Error(err), zap.String("listen_addr", pprofListenAddr))
		}
	}()
}
