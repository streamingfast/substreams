package reqctx

import "context"

type Tier2RequestParameters struct {
	MeteringConfig       string
	FirstStreamableBlock uint64

	MergedBlockStoreURL  string
	StateStoreURL        string
	StateBundleSize      uint64
	StateStoreDefaultTag string

	BlockType string

	WASMModules map[string]string
}

type tier2RequestParametersKey int

const tier2RequestParametersKeyKey = tier2RequestParametersKey(0)

func WithTier2RequestParameters(ctx context.Context, parameters Tier2RequestParameters) context.Context {
	return context.WithValue(ctx, tier2RequestParametersKeyKey, parameters)
}

func GetTier2RequestParameters(ctx context.Context) (Tier2RequestParameters, bool) {
	parameters, ok := ctx.Value(tier2RequestParametersKeyKey).(Tier2RequestParameters)
	return parameters, ok
}
