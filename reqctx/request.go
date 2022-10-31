package reqctx

import pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"

type RequestDetails struct {
	Request                *pbsubstreams.Request
	EffectiveStartBlockNum uint64
	IsSubRequest           bool

	StartHistoricalBlock uint64 // block at which we start sending
	StartLiveBlockNum    uint64 // block at which we hand off to live

}

//type Request struct {
//	StartCursor          string
//	StartHistoricalBlock uint64 // block at which we start sending
//	StartLiveBlockNum    uint64 // block at which we hand off to live
//	StopBlockNum         uint64 // block at which we stop (excluded)
//
//	LiveForkSteps []pbsubstreams.ForkStep
//	Modules       *pbsubstreams.Modules
//
//	OutputModules                             []string
//	SendStoreSnapshotAtLiveBoundaryForModules []string
//
//	// Keep reference to ModuleTree?!
//
//	IsSubRequest bool
//}
