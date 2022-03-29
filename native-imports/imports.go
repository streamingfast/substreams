package imports

import (
	"github.com/streamingfast/bstream"
	"github.com/streamingfast/eth-go/rpc"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/ethereum/substreams/v1"
	ssrpc "github.com/streamingfast/substreams/rpc"
)

type Imports struct {
	rpcCache  *ssrpc.Cache
	rpcClient *rpc.Client

	currentBlock bstream.BlockRef
}

func NewImports(rpcClient *rpc.Client, rpcCache *ssrpc.Cache) *Imports {
	return &Imports{
		rpcClient: rpcClient,
		rpcCache:  rpcCache,
	}
}

func (i *Imports) SetCurrentBlock(ref bstream.BlockRef) {
	i.currentBlock = ref
}

func (i *Imports) RPC(calls *pbsubstreams.RpcCalls) *pbsubstreams.RpcResponses {
	return ssrpc.RPCCalls(
		i.currentBlock.Num(),
		i.rpcClient,
		i.rpcCache,
		calls,
	)
}
