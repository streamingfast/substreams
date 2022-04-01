package test

import (
	"encoding/hex"
	"encoding/json"
	"io/ioutil"
	"os"
	"testing"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/ethereum/substreams/v1"
	"github.com/streamingfast/substreams/pipeline"
	"github.com/streamingfast/substreams/wasm"
	"github.com/test-go/testify/assert"
	"github.com/test-go/testify/require"
)

func TestRPCCalls(t *testing.T) {
	cases := []struct {
		wasmFile      string
		functionName  string
		expectCalls   []*pbsubstreams.RpcCalls
		nextResponses []*pbsubstreams.RpcResponses
	}{
		{
			wasmFile:     "./target/wasm32-unknown-unknown/release/testing_substreams.wasm",
			functionName: "test_eth_call",
			expectCalls: []*pbsubstreams.RpcCalls{
				{
					Calls: []*pbsubstreams.RpcCall{
						{
							ToAddr:          mustDecodeHexString("EA674fdDe714fd979de3EdF0F56AA9716B898ec8"),
							MethodSignature: mustDecodeHexString("deadbeef"),
						},
					},
				},
			},
			nextResponses: []*pbsubstreams.RpcResponses{
				nil,
			},
		},
		{
			wasmFile:     "./target/wasm32-unknown-unknown/release/testing_substreams.wasm",
			functionName: "test_eth_call_2",
			expectCalls: []*pbsubstreams.RpcCalls{
				{
					Calls: []*pbsubstreams.RpcCall{
						{
							ToAddr:          mustDecodeHexString("EA674fdDe714fd979de3EdF0F56AA9716B898ec8"),
							MethodSignature: mustDecodeHexString("deadbeef"),
						},
						{
							ToAddr:          mustDecodeHexString("0e09fabb73bd3ade0a17ecc321fd13a19e81ce82"),
							MethodSignature: mustDecodeHexString("beefdead"),
						},
					},
				},
			},
			nextResponses: []*pbsubstreams.RpcResponses{
				nil,
			},
		},
		{
			wasmFile:     "./target/wasm32-unknown-unknown/release/testing_substreams.wasm",
			functionName: "test_eth_call_3",
			expectCalls: []*pbsubstreams.RpcCalls{
				{
					Calls: []*pbsubstreams.RpcCall{
						{
							ToAddr:          mustDecodeHexString("EA674fdDe714fd979de3EdF0F56AA9716B898ec8"),
							MethodSignature: mustDecodeHexString("deadbeef"),
						},
						{
							ToAddr:          mustDecodeHexString("0e09fabb73bd3ade0a17ecc321fd13a19e81ce82"),
							MethodSignature: mustDecodeHexString("beefdead"),
						},
						{
							ToAddr:          mustDecodeHexString("d006a7431be66fec522503db41f54692b85447c1"),
							MethodSignature: mustDecodeHexString("feebdead"),
						},
					},
				},
			},
			nextResponses: []*pbsubstreams.RpcResponses{
				nil,
			},
		},
	}
	for _, c := range cases {
		t.Run(c.functionName, func(t *testing.T) {
			file, err := os.Open(c.wasmFile)
			require.NoError(t, err)
			byteCode, err := ioutil.ReadAll(file)
			require.NoError(t, err)
			module, err := wasm.NewModule(byteCode, c.functionName)
			require.NoError(t, err)

			rpcProv := &testRpcProvider{
				nextResponses: c.nextResponses,
			}

			instance, err := module.NewInstance(c.functionName, nil, pipeline.GetRPCWasmFunctionFactory(rpcProv, module))
			require.NoError(t, err)
			err = instance.Execute()
			require.NoError(t, err)

			assert.JSONEq(t, toJSON(c.expectCalls), toJSON(rpcProv.calls))
		})
	}
}

func toJSON(in interface{}) string {
	out, err := json.Marshal(in)
	if err != nil {
		panic(err)
	}
	return string(out)
}

type testRpcProvider struct {
	calls         []*pbsubstreams.RpcCalls
	nextResponses []*pbsubstreams.RpcResponses
}

func (i *testRpcProvider) RPC(calls *pbsubstreams.RpcCalls) (out *pbsubstreams.RpcResponses) {
	i.calls = append(i.calls, calls)
	out, i.nextResponses = i.nextResponses[0], i.nextResponses[1:]
	return
}

func mustDecodeHexString(in string) []byte {
	out, err := hex.DecodeString(in)
	if err != nil {
		panic(err)
	}
	return out
}
