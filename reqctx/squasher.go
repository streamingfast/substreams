package reqctx

import (
	"context"

	"github.com/streamingfast/substreams/squash"
)

var remoteSquasherKey = contextKeyType(17)

// RemoteSquasher is the per-request handle tier1 uses to call a remote squash
// implementation. Nil (or a context without one) means squash in-process.
// When set, each squash run is one Squash RPC per store module, covering every
// claimed segment, so tier1 does not read store files for merging.
type RemoteSquasher struct {
	Client         squash.Client
	StateStoreURL  string
	CacheTag       string
	StoreSizeLimit uint64
}

func WithRemoteSquasher(ctx context.Context, squasher *RemoteSquasher) context.Context {
	return context.WithValue(ctx, remoteSquasherKey, squasher)
}

func GetRemoteSquasher(ctx context.Context) *RemoteSquasher {
	val := ctx.Value(remoteSquasherKey)
	if s, ok := val.(*RemoteSquasher); ok {
		return s
	}
	return nil
}
