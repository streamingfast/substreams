package reqctx

import "context"

// WithStoreSizeLimit stores a per-request store size limit override in the context.
// When non-zero, this value overrides the default store.StoreSizeLimit for stores
// loaded during a tier2 request.
func WithStoreSizeLimit(ctx context.Context, limit uint64) context.Context {
	return context.WithValue(ctx, storeSizeLimitKey, limit)
}

// StoreSizeLimit returns the per-request store size limit from the context, or 0
// if no override was set (meaning the default store.StoreSizeLimit should be used).
func StoreSizeLimit(ctx context.Context) uint64 {
	limit, ok := ctx.Value(storeSizeLimitKey).(uint64)
	if !ok {
		return 0
	}
	return limit
}
