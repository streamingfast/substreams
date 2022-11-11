package integration

import (
	"github.com/streamingfast/bstream"
	"github.com/streamingfast/dstore"
	"github.com/streamingfast/substreams/storage/store"
)

type blockProcessedCallBack func(ctx *execContext)

type execContext struct {
	block     *bstream.Block
	stores    store.Map
	baseStore dstore.Store
}
