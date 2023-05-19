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
	StopBlockNum          uint64
	UniqueID              uint64

	ProductionMode bool
	IsSubRequest   bool
}

func (d *RequestDetails) UniqueIDString() string {
	return strconv.FormatUint(d.UniqueID, 10)
}

func (d *RequestDetails) IsOutputModule(modName string) bool {
	return modName == d.OutputModule
}

// Called to determine if we *really* need to save this store snapshot. We don't need
// when we're doing parallel processing and we are concerned only with writing the
// leaf stores we've been asked to produce.  We know the scheduler will have
// created jobs to produce those stores we're skipping here.
func (d *RequestDetails) SkipSnapshotSave(modName string) bool {
	return d.IsSubRequest && !d.IsOutputModule(modName)
}

func (d *RequestDetails) ShouldReturnWrittenPartials(modName string) bool {
	return d.IsSubRequest && d.IsOutputModule(modName)
}

func (d *RequestDetails) ShouldReturnProgressMessages() bool {
	return d.IsSubRequest
}

func (d *RequestDetails) ShouldStreamCachedOutputs() bool {
	return d.ProductionMode &&
		d.ResolvedStartBlockNum < d.LinearHandoffBlockNum
}
