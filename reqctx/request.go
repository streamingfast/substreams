package reqctx

import pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"

type RequestDetails struct {
	Request *pbsubstreams.Request

	EffectiveStartBlockNum uint64 // TODO(abourget): to be replaced by LiveHandoffBlockNum

	// Stream
	StartHistoricalBlock uint64 // block at which we start sending
	// or LiveHandoffBlockNum
	StartLiveBlockNum uint64 // block at which we hand off to live

	StopBlockNum uint64

	IsSubRequest   bool
	IsOutputModule map[string]bool
}

func (d *RequestDetails) SkipSnapshotSave(modName string) bool {
	// optimization because we know that in a subrequest we are only running through the last store (output)
	// all parent stores should have come from moduleOutput cache
	return d.IsSubRequest && d.IsOutputModule[modName]
}

func (d *RequestDetails) ShouldReturnWrittenPartialsInTrailer(modName string) bool {
	return d.IsSubRequest && d.IsOutputModule[modName]
}

func (d *RequestDetails) ShouldReturnProgressMessages() bool {
	return d.IsSubRequest
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
