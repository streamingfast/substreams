package reqctx

import pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"

type Request struct {
	StartCursor          string
	StartHistoricalBlock uint64 // block at which we start sending
	StartLiveBlockNum    uint64 // block at which we hand off to live
	StopBlockNum         uint64 // block at which we stop (excluded)

	LiveForkSteps []pbsubstreams.ForkStep
	Modules       *pbsubstreams.Modules

	OutputModules                             []string
	SendStoreSnapshotAtLiveBoundaryForModules []string

	IsSubrequest bool
}
