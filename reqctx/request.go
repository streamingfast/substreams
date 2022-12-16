package reqctx

import pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"

type IsOutputModuleFunc func(name string) bool

type RequestDetails struct {
	Request *pbsubstreams.Request

	// What the user requested, derived from either the Request.StartBlockNum or Request.Cursor
	RequestStartBlockNum  uint64
	LinearHandoffBlockNum uint64
	StopBlockNum          uint64

	IsSubRequest   bool
	IsOutputModule IsOutputModuleFunc
}

// Called to determine if we *really* need to save this store snapshot. We don't need
// when we're doing parallel processing and we are concerned only with writing the
// leaf stores we've been asked to produce.  We know the scheduler will have
// created jobs to produce those stores we're skipping here.
func (d *RequestDetails) SkipSnapshotSave(modName string) bool {
	return d.IsSubRequest && !d.IsOutputModule(modName)
}

func (d *RequestDetails) ShouldReturnWrittenPartialsInTrailer(modName string) bool {
	return d.IsSubRequest && d.IsOutputModule(modName)
}

func (d *RequestDetails) ShouldReturnProgressMessages() bool {
	return d.IsSubRequest
}

func (d *RequestDetails) ShouldStreamCachedOutputs() bool {
	return d.Request.ProductionMode &&
		d.RequestStartBlockNum < d.LinearHandoffBlockNum
}
