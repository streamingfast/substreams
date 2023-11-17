package integration

import (
	"github.com/streamingfast/dstore"
	pbbstream "github.com/streamingfast/pbgo/sf/bstream/v1"
	"github.com/streamingfast/substreams/storage/store"
)

type blockProcessedCallBack func(ctx *execContext)

type execContext struct {
	block     *pbbstream.Block
	stores    store.Map
	baseStore dstore.Store
}
