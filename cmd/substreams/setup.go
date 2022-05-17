package main

import (
	"net/http"
	_ "net/http/pprof"

	"go.uber.org/zap"
)

func setup() {
	setupProfiler()
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
