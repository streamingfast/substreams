package work

import (
	"context"
	"testing"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/reqctx"
	"github.com/stretchr/testify/assert"
)

func TestNewRequestCarriesMergedBlocksBundleSize(t *testing.T) {
	ctx := context.Background()
	ctx = reqctx.WithTier2RequestParameters(ctx, reqctx.Tier2RequestParameters{
		StateBundleSize:        1000,
		MergedBlocksBundleSize: 1000,
	})

	req := NewRequest(ctx, &reqctx.RequestDetails{OutputModule: "m", Modules: &pbsubstreams.Modules{}}, 0, 2000, false)
	assert.Equal(t, uint64(1000), req.MergedBlocksBundleSize)
	assert.Equal(t, uint64(1000), req.MergedBlocksBundleSizeOrDefault())

	// older tier1s (or unset config) leave the field at 0 => default 100
	ctx = reqctx.WithTier2RequestParameters(context.Background(), reqctx.Tier2RequestParameters{
		StateBundleSize: 1000,
	})
	req = NewRequest(ctx, &reqctx.RequestDetails{OutputModule: "m", Modules: &pbsubstreams.Modules{}}, 0, 2000, false)
	assert.Equal(t, uint64(0), req.MergedBlocksBundleSize)
	assert.Equal(t, uint64(100), req.MergedBlocksBundleSizeOrDefault())
}
