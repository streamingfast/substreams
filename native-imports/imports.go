package imports

import (
	"github.com/streamingfast/bstream"
	"github.com/streamingfast/eth-go/rpc"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/ethereum/substreams/v1"
	ssrpc "github.com/streamingfast/substreams/rpc"
)

type Imports struct {
	rpcClient *rpc.Client

	currentBlock bstream.BlockRef
}

func NewImports(rpcClient *rpc.Client) *Imports {
	return &Imports{
		rpcClient: rpcClient,
	}
}

func (i *Imports) SetCurrentBlock(ref bstream.BlockRef) {
	i.currentBlock = ref
}

func (i *Imports) RPC(calls *pbsubstreams.RpcCalls) *pbsubstreams.RpcResponses {
	return ssrpc.RPCCalls(
		i.currentBlock.Num(),
		i.rpcClient,
		calls,
	)
}
