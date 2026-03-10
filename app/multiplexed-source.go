package app

import (
	"context"
	"time"

	"github.com/streamingfast/bstream"
	"github.com/streamingfast/bstream/blockstream"
	"go.uber.org/zap"
)

func NewMultiplexedSource(handler bstream.Handler, sourceAddresses []string, maxSourceLatency time.Duration, sourceRequestBurst int, logger *zap.Logger) bstream.Source {
	ctx := context.Background()

	var sourceFactories []bstream.SourceFactory
	for _, u := range sourceAddresses {

		url := u
		sf := func(subHandler bstream.Handler) bstream.Source {
			gate := bstream.NewRealtimeGate(maxSourceLatency, subHandler, bstream.GateOptionWithLogger(logger))
			var upstreamHandler bstream.Handler
			upstreamHandler = bstream.HandlerFunc(gate.ProcessBlock)

			src := blockstream.NewSource(ctx, url, int64(sourceRequestBurst), upstreamHandler, blockstream.WithLogger(logger), blockstream.WithRequester("substreams-tier1"), blockstream.WithPartialBlocks())
			return src
		}
		sourceFactories = append(sourceFactories, sf)
	}

	return bstream.NewMultiplexedSource(sourceFactories, handler, bstream.MultiplexedSourceWithLogger(logger))
}
