package reqctx

import (
	"context"
	"fmt"
	"os"
	"time"

	"go.uber.org/zap"
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

// EnvEthCallFallbackToLatestDuration is the environment variable name that, when set,
// enables a non-deterministic fallback to latest for Ethereum eth_call/eth_getBalance.
// Its presence requires callers to acknowledge non-determinism via the
// X-substreams-acknowledge-non-deterministic header.
const EnvEthCallFallbackToLatestDuration = "ETH_CALL_FALLBACK_TO_LATEST_DURATION"

func WithEnvEthCallFallbackToLatestDuration(ctx context.Context, logger *zap.Logger, fallbackDuration time.Duration) context.Context {
	duration := fallbackDuration
	if value := os.Getenv(EnvEthCallFallbackToLatestDuration); value != "" {
		var err error
		duration, err = time.ParseDuration(value)
		if err != nil {
			panic(fmt.Errorf("invalid value for env var %s: %w", value, err))
		}
	}

	return WithEthCallFallbackToLatestDuration(ctx, duration)
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

func HasEthCallFallbackToLatestDuration(ctx context.Context) bool {
	return EthCallFallbackToLatestDuration(ctx) > 0
}

// EnvEthCallUseBlockNumberDuration is the environment variable name that, when set,
// enables using block number instead of block hash for Ethereum eth_call/eth_getBalance after a specified duration.
// This is to be used on chains with deterministic behavior but with an archive node that needs the block number to route old queries.
const EnvEthCallUseBlockNumberDuration = "ETH_CALL_USE_BLOCK_NUMBER_DURATION"

func WithEnvEthCallUseBlockNumberDuration(ctx context.Context, logger *zap.Logger, fallbackDuration time.Duration) context.Context {
	duration := fallbackDuration
	if envValue := os.Getenv(EnvEthCallUseBlockNumberDuration); envValue != "" {
		var err error
		duration, err = time.ParseDuration(envValue)
		if err != nil {
			panic(fmt.Errorf("invalid value for env var %s: %w", envValue, err))
		}
	}

	return WithEthCallUseBlockNumberDuration(ctx, duration)
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
