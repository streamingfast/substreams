package rpc

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/streamingfast/eth-go"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/ethereum/substreams/v1"

	rpc "github.com/streamingfast/eth-go/rpc"
	"go.uber.org/zap"
)

type RPCCall struct {
	ToAddr          string
	MethodSignature string // ex: "name() (string)"
}

func (c *RPCCall) ToString() string {
	return fmt.Sprintf("%s:%s", c.ToAddr, c.MethodSignature)
}

type RPCResponse struct {
	Decoded       []interface{}
	Raw           string
	DecodingError error
	CallError     error // always deterministic
}

func RPCCalls(blockNum uint64, rpcClient *rpc.Client, rpcCache *Cache, calls *pbsubstreams.RpcCalls) (out *pbsubstreams.RpcResponses) {
	var reqs []*rpc.RPCRequest
	for _, call := range calls.Calls {
		req := &rpc.RPCRequest{
			Params: []interface{}{
				map[string]interface{}{
					"to":   eth.Hex(call.ToAddr).Pretty(),
					"data": eth.Hex(call.MethodSignature).Pretty(),
				},
				blockNum,
			},
			Method: "eth_call",
		}
		reqs = append(reqs, req)
	}

	var cacheKey CacheKey
	if rpcCache != nil {
		var cacheKeyParts []interface{}
		cacheKeyParts = append(cacheKeyParts, blockNum)
		for _, call := range calls.Calls {
			cacheKeyParts = append(cacheKeyParts, callToString(call))
		}
		cacheKey = rpcCache.Key("rpc", cacheKeyParts...)

		if fromCache, found := rpcCache.GetRaw(cacheKey); found {
			rpcResp := []*rpc.RPCResponse{}
			err := json.Unmarshal(fromCache, &rpcResp)
			if err != nil {
				zlog.Warn("cannot unmarshal Cache response for rpc call", zap.Error(err))
			} else {
				for i, resp := range rpcResp {
					resp.CopyDecoder(reqs[i])
				}
				resps := toProtoResponses(rpcResp)
				return resps
			}
		}
	}

	ctx := context.Background()
	var delay time.Duration
	var attemptNumber int
	for {
		time.Sleep(delay)

		attemptNumber += 1
		delay = minDuration(time.Duration(attemptNumber*500)*time.Millisecond, 10*time.Second)

		out, err := rpcClient.DoRequests(ctx, reqs)
		if err != nil {
			zlog.Warn("retrying RPCCall on RPC error", zap.Error(err), zap.Uint64("at_block", blockNum))
			continue
		}

		var nonDeterministicResp bool
		for _, resp := range out {
			if !resp.Deterministic() {
				zlog.Warn("retrying RPCCall on non-deterministic RPC call error", zap.Error(resp.Err), zap.Uint64("at_block", blockNum))
				nonDeterministicResp = true
				break
			}
		}
		if nonDeterministicResp {
			continue
		}
		if rpcCache != nil {
			rpcCache.Set(cacheKey, out)
		}
		resp := toProtoResponses(out)
		return resp
	}
}

// ToProtoCalls is a wrapper for previous format
func ToProtoCalls(in []*RPCCall) (out *pbsubstreams.RpcCalls) {
	for _, call := range in {
		methodSig := eth.MustNewMethodDef(call.MethodSignature).MethodID()
		toAddr := eth.MustNewAddress(call.ToAddr)
		out.Calls = append(out.Calls, &pbsubstreams.RpcCall{
			ToAddr:          toAddr,
			MethodSignature: methodSig,
		})
	}
	return
}

func toProtoResponses(in []*rpc.RPCResponse) (out *pbsubstreams.RpcResponses) {
	out = &pbsubstreams.RpcResponses{}
	for _, resp := range in {
		newResp := &pbsubstreams.RpcResponse{}
		if resp.Err != nil {
			newResp.Failed = true
		} else {
			if !strings.HasPrefix(resp.Content, "0x") {
				newResp.Failed = true
			} else {
				bytes, err := hex.DecodeString(resp.Content[2:])
				if err != nil {
					newResp.Failed = true
				} else {
					newResp.Raw = bytes
				}
			}
		}
		out.Responses = append(out.Responses, newResp)
	}
	return
}

func callToString(c *pbsubstreams.RpcCall) string {
	return fmt.Sprintf("%x:%x", c.ToAddr, c.MethodSignature)
}

func toRPCResponse(in []*rpc.RPCResponse) (out []*RPCResponse) {
	for _, rpcResp := range in {
		decoded, decodingError := rpcResp.Decode()
		out = append(out, &RPCResponse{
			Decoded:       decoded,
			DecodingError: decodingError,
			CallError:     rpcResp.Err,
			Raw:           rpcResp.Content,
		})
	}
	return
}

func minDuration(a, b time.Duration) time.Duration {
	if a <= b {
		return a
	}
	return b
}
