package imports

import (
	"context"
	"math"

	"github.com/streamingfast/bstream"
	"github.com/streamingfast/eth-go/rpc"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/ethereum/substreams/v1"
	ssrpc "github.com/streamingfast/substreams/rpc"
)

type Imports struct {
	rpcCacheManager *ssrpc.CacheManager
	rpcClient       *rpc.Client

	currentBlock bstream.BlockRef
}

func NewImports(rpcClient *rpc.Client, rpcCacheManager *ssrpc.CacheManager) *Imports {
	return &Imports{
		rpcClient:       rpcClient,
		rpcCacheManager: rpcCacheManager,
	}
}

func (i *Imports) SetCurrentBlock(ref bstream.BlockRef) {
	i.currentBlock = ref
}

func (i *Imports) RPC(calls *pbsubstreams.RpcCalls) *pbsubstreams.RpcResponses {
	return ssrpc.RPCCalls(
		i.currentBlock.Num(),
		i.rpcClient,
		i.rpcCacheManager.Get(context.Background(), i.currentBlock.Num(), math.MaxUint64),
		calls,
	)
}
