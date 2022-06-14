package orchestrator

import (
	"fmt"
	"strings"

	"go.uber.org/zap/zapcore"

	"github.com/streamingfast/substreams/block"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

// FIXME(abourget): WorkPlan ?

type SplitWorkModules map[string]*WorkUnit

func (mods SplitWorkModules) ProgressMessages() (out []*pbsubstreams.ModuleProgress) {
	for storeName, work := range mods {
		if work.completedRange == nil {
			continue
		}
		out = append(out, &pbsubstreams.ModuleProgress{
			Name: storeName,
			Type: &pbsubstreams.ModuleProgress_ProcessedRanges{
				ProcessedRanges: &pbsubstreams.ModuleProgress_ProcessedRange{
					ProcessedRanges: []*pbsubstreams.BlockRange{
						{
							StartBlock: work.completedRange.StartBlock,
							EndBlock:   work.completedRange.ExclusiveEndBlock,
						},
					},
				},
			},
		})
	}
	return
}

type WorkUnit struct {
	modName string

	completedRange *block.Range // Send a Progress message, saying the store is already processed for this range

	partialsMissing block.Ranges
	partialsPresent block.Ranges

	subRequestSplitSize uint64
	RequestRanges       block.Ranges
}

func (work *WorkUnit) InitialProcessedPartials() block.Ranges {
	return work.partialsPresent.Merged()
}

func SplitWork(modName string, subRequestSlipSize, modInitBlock, incomingReqStartBlock uint64, snapshots *Snapshots) (work *WorkUnit) {
	work = &WorkUnit{modName: modName, subRequestSplitSize: subRequestSlipSize}

	if incomingReqStartBlock <= modInitBlock {
		return work
	}

	storeLastComplete := snapshots.LastCompletedBlockBefore(incomingReqStartBlock)

	if storeLastComplete != 0 && storeLastComplete <= modInitBlock {
		panic("cannot have saved last store before module's init block") // 0 has special meaning
	}

	backProcessStartBlock := modInitBlock
	if storeLastComplete != 0 {
		backProcessStartBlock = storeLastComplete
		work.completedRange = block.NewRange(modInitBlock, storeLastComplete)
	}

	if storeLastComplete == incomingReqStartBlock {
		return
	}

	for ptr := backProcessStartBlock; ptr < incomingReqStartBlock; {
		end := minOf(ptr-ptr%subRequestSlipSize+subRequestSlipSize, incomingReqStartBlock)
		newPartial := block.NewRange(ptr, end)
		if !snapshots.ContainsPartial(newPartial) {
			work.partialsMissing = append(work.partialsMissing, newPartial)
		} else {
			work.partialsPresent = append(work.partialsPresent, newPartial)
		}
		ptr = end
	}
	work.RequestRanges = work.partialsMissing.MergeRanges(work.subRequestSplitSize)
	return work
}

func minOf(a, b uint64) uint64 {
	if a < b {
		return a
	}
	return b
}

type chunks []*chunk

func (c chunks) String() string {
	var sc []string
	for _, s := range c {
		var add string
		if s.tempPartial {
			add = "TMP:"
		}
		sc = append(sc, fmt.Sprintf("%s%d-%d", add, s.start, s.end))
	}
	return strings.Join(sc, ", ")
}

type chunk struct {
	start       uint64
	end         uint64 // exclusive end
	tempPartial bool   // for off-of-bound stores (like ending in 1123, and not on 1000)
}

func (c *chunk) String() string {
	var add string
	if c.tempPartial {
		add = "TMP:"
	}
	return fmt.Sprintf("%s%d-%d", add, c.start, c.end)
}
func (c *chunk) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	enc.AddUint64("start_block", c.start)
	enc.AddUint64("end_block", c.end)

	return nil
}
