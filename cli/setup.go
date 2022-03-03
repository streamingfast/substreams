package cli

import (
	"net/http"

	"go.uber.org/zap"
)

func setup() {
	setupProfiler()
}

func setupProfiler() {
	go func() {
		err := http.ListenAndServe(pprofListenAddr, nil)
		if err != nil {
			zlog.Debug("unable to start profiling server", zap.Error(err), zap.String("listen_addr", pprofListenAddr))
		}
	}()
}
