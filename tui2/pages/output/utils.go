package output

import pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"

func isEmptyModuleOutput(in *pbsubstreams.ModuleOutput) bool {
	switch d := in.Data.(type) {
	case *pbsubstreams.ModuleOutput_MapOutput:
		return len(d.MapOutput.Value) == 0
	case *pbsubstreams.ModuleOutput_DebugStoreDeltas:
		return len(d.DebugStoreDeltas.Deltas) == 0
	}
	return true
}
