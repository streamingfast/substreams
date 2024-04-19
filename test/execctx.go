package integration

import (
	pbbstream "github.com/streamingfast/bstream/pb/sf/bstream/v1"
	"github.com/streamingfast/dstore"
	"github.com/streamingfast/substreams/storage/store"
)

type blockProcessedCallBack func(ctx *execContext)

type execContext struct {
	block     *pbbstream.Block
	stores    store.Map
	baseStore dstore.Store
}
