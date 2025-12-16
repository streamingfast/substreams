package reqctx

import (
	"context"
	"time"
)

type Tier2RequestParameters struct {
	MeteringConfig       string
	FirstStreamableBlock uint64

	MergedBlockStoreURL  string
	StateStoreURL        string
	StateBundleSize      uint64
	StateStoreDefaultTag string

	BlockType string

	WASMModules                map[string]string
	FoundationalStoreEndpoints map[string]string
}

func WithTier2RequestParameters(ctx context.Context, parameters Tier2RequestParameters) context.Context {
	return context.WithValue(ctx, tier2RequestParametersKey, parameters)
}

func GetTier2RequestParameters(ctx context.Context) (Tier2RequestParameters, bool) {
	parameters, ok := ctx.Value(tier2RequestParametersKey).(Tier2RequestParameters)
	return parameters, ok
}

func WithEthCallFallbackToLatestDuration(ctx context.Context, duration time.Duration) context.Context {
	return context.WithValue(ctx, ethCallFallbackToLatestDuration, duration)
}

func EthCallFallbackToLatestDuration(ctx context.Context) time.Duration {
	duration, ok := ctx.Value(ethCallFallbackToLatestDuration).(time.Duration)
	if !ok {
		return time.Duration(0)
	}
	return duration
}

func WithEthCallUseBlockNumberDuration(ctx context.Context, duration time.Duration) context.Context {
	return context.WithValue(ctx, ethCallUseBlockNumberDuration, duration)
}

func EthCallUseBlockNumberDuration(ctx context.Context) time.Duration {
	duration, ok := ctx.Value(ethCallUseBlockNumberDuration).(time.Duration)
	if !ok {
		return time.Duration(0)
	}
	return duration
}
