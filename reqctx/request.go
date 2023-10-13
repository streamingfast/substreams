package reqctx

import (
	"strconv"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

type IsOutputModuleFunc func(name string) bool

type RequestDetails struct {
	Modules *pbsubstreams.Modules

	DebugInitialStoreSnapshotForModules []string
	OutputModule                        string
	// What the user requested, derived from either the Request.StartBlockNum or Request.Cursor
	ResolvedStartBlockNum uint64
	ResolvedCursor        string

	LinearHandoffBlockNum uint64
	LinearGateBlockNum    uint64
	StopBlockNum          uint64
	MaxParallelJobs       uint64
	CacheTag              string
	UniqueID              uint64

	ProductionMode bool
	IsTier2Request bool
	Tier2Stage     int
}

func (d *RequestDetails) UniqueIDString() string {
	return strconv.FormatUint(d.UniqueID, 10)
}

func (d *RequestDetails) IsOutputModule(modName string) bool {
	return modName == d.OutputModule
}

func (d *RequestDetails) ShouldReturnWrittenPartials(modName string) bool {
	return d.IsTier2Request && d.IsOutputModule(modName)
}

func (d *RequestDetails) ShouldStreamCachedOutputs() bool {
	return d.ProductionMode &&
		d.ResolvedStartBlockNum < d.LinearHandoffBlockNum
}
