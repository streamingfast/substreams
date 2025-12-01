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
	return context.WithValue(ctx, tier2RequestParametersKeyKey, parameters)
}

func WithEthCallFallbackToLatestDuration(ctx context.Context, duration time.Duration) context.Context {
	return context.WithValue(ctx, tier2RequestParametersKeyKey, duration)
}

func EthCallFallbackToLatestDuration(ctx context.Context) time.Duration {
	duration, ok := ctx.Value(tier2RequestParametersKeyKey).(time.Duration)
	if !ok {
		return time.Duration(0)
	}
	return duration
}

func GetTier2RequestParameters(ctx context.Context) (Tier2RequestParameters, bool) {
	parameters, ok := ctx.Value(tier2RequestParametersKeyKey).(Tier2RequestParameters)
	return parameters, ok
}
