package output

import pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"

func isEmptyMapModuleOutput(in *pbsubstreamsrpc.MapModuleOutput) bool {
	return len(in.MapOutput.Value) == 0
}

func isEmptyStoreModuleOutput(in *pbsubstreamsrpc.StoreModuleOutput) bool {
	return len(in.DebugStoreDeltas) == 0
}
