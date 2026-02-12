package reqctx

import (
	"fmt"
	"strconv"
	"time"

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

	LinearHandoffBlockNum         uint64
	LinearGateBlockNum            uint64
	StopBlockNum                  uint64
	MaxParallelJobs               uint64
	MaxStageLayerParallelExecutor uint64
	LimitProcessedBlocks          uint64
	UpdateInterval                time.Duration
	UniqueID                      uint64

	FromQuickload    bool
	ProductionMode   bool
	IsTier2Request   bool
	IsStreamingTier2 bool // special mode where tier2 will stream the data back to tier1, for the first segment
	Tier2Stage       int
}

func (d *RequestDetails) AssertProcessedBlocksLimit(requiredBlocksStore, requiredBlocksRange uint64) error {
	if d.LimitProcessedBlocks == 0 {
		return nil
	}
	if requiredBlocksStore+requiredBlocksRange > d.LimitProcessedBlocks {
		return fmt.Errorf("request needs to process a total of %d blocks (including %d to prepare the stores) but only %d blocks are allowed according to the 'limit-processed-blocks' request argument", requiredBlocksStore+requiredBlocksRange, requiredBlocksStore, d.LimitProcessedBlocks)
	}
	return nil
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
