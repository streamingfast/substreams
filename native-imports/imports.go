package imports

import (
	"github.com/streamingfast/bstream"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/ethereum/substreams/v1"
)

type Imports struct {
	currentBlock bstream.BlockRef
}

func NewImports() *Imports {
	return &Imports{}
}

func (i *Imports) SetCurrentBlock(ref bstream.BlockRef) {
	i.currentBlock = ref
}

func (i *Imports) RPC(calls *pbsubstreams.RpcCalls) *pbsubstreams.RpcResponses {
	// We NOW have only WASM extensions, so when we'll want to have
	// native code support for some stuff, it won't be for extensions,
	// but only for high speed in-memory manipulations of data
	// structures.
	return nil
}
