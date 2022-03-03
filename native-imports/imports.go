package imports

import (
	"github.com/streamingfast/bstream"
	"github.com/streamingfast/eth-go/rpc"
	ssrpc "github.com/streamingfast/substreams/rpc"
)

type Imports struct {
	rpcCache      *ssrpc.Cache
	rpcClient     *rpc.Client
	noArchiveMode bool

	currentBlock bstream.BlockRef
}

func NewImports(rpcClient *rpc.Client, rpcCache *ssrpc.Cache, noArchiveMode bool) *Imports {
	return &Imports{
		rpcClient:     rpcClient,
		rpcCache:      rpcCache,
		noArchiveMode: noArchiveMode,
	}
}
func (s *Imports) SetCurrentBlock(ref bstream.BlockRef) {
	s.currentBlock = ref
}

func (i *Imports) RPC(calls []*ssrpc.RPCCall) ([]*ssrpc.RPCResponse, error) {
	return ssrpc.DoRPCCalls(i.noArchiveMode, i.currentBlock.Num(), i.rpcClient, i.rpcCache, calls)
}
