package distributor

import (
	"context"
	"io"
	"iter"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/reqctx"
	"github.com/streamingfast/substreams/storage/execout"
	"github.com/streamingfast/substreams/storage/index"
	"go.uber.org/zap"
)

type ClockDistributor struct {
	execOuts           map[string]execout.FileReader
	execOutsLastclock  map[string]uint64
	seenClocks         map[uint64]*pbsubstreams.Clock
	startBlock         uint64
	stopBlock          uint64
	nextClockNumber    uint64
	outputModuleInputs []*pbsubstreams.Module_Input
	// moduleBlockIndexes maps a module name to its *index.BlockIndex (if any).
	// A module that is present in the execution plan but has no block index will
	// have a nil value stored here, while modules that are not part of the plan
	// at all will be absent from the map.
	moduleBlockIndexes map[string]*index.BlockIndex
}

func NewClockDistributor(
	execOuts map[string]execout.FileReader,
	startBlock uint64,
	stopBlock uint64,
	outputModuleInputs []*pbsubstreams.Module_Input,
	moduleBlockIndexes map[string]*index.BlockIndex,
) *ClockDistributor {
	return &ClockDistributor{
		execOuts:           execOuts,
		execOutsLastclock:  make(map[string]uint64),
		seenClocks:         make(map[uint64]*pbsubstreams.Clock),
		startBlock:         startBlock,
		stopBlock:          stopBlock,
		nextClockNumber:    startBlock,
		outputModuleInputs: outputModuleInputs,
		moduleBlockIndexes: moduleBlockIndexes,
	}
}

// canSkipBlockForModule returns true when the module's block index exists
// and reports that block i can be skipped. If the module has no block index,
// the block cannot be skipped.
func (cd *ClockDistributor) canSkipBlockForModule(moduleName string, i uint64, logger *zap.Logger) bool {
	blockIndex, ok := cd.moduleBlockIndexes[moduleName]
	if !ok {
		logger.Debug("module block index not found in canSkipBlockForModule", zap.String("module", moduleName))
	}
	if blockIndex == nil {
		// If the module has no index, we cannot skip the block.
		return false
	}
	return blockIndex.Skip(i)
}

func (cd *ClockDistributor) Next(ctx context.Context) (*pbsubstreams.Clock, error) {
	logger := reqctx.Logger(ctx)
	for i := cd.nextClockNumber; i < cd.stopBlock; i++ {
		indicesSkip := true
		if len(cd.moduleBlockIndexes) > 0 {
			for _, input := range cd.outputModuleInputs {

				if source := input.GetSource(); source != nil {
					indicesSkip = false
					break
				}

				if store := input.GetStore(); store != nil {
					if store.GetMode() == pbsubstreams.Module_Input_Store_DELTAS {
						if !cd.canSkipBlockForModule(store.ModuleName, i, logger) {
							indicesSkip = false
							break
						}
					}
				}

				if m := input.GetMap(); m != nil {
					if !cd.canSkipBlockForModule(m.ModuleName, i, logger) {
						indicesSkip = false
						break
					}
				}

			}
		} else {
			indicesSkip = false
		}
		if indicesSkip {
			cd.nextClockNumber = i + 1
			continue
		}

		for name, execOut := range cd.execOuts {
			for {
				lastRead, ok := cd.execOutsLastclock[name]
				if ok && lastRead >= i {
					break
				}
				item, err := execOut.ReadNext()
				if err == io.EOF {
					cd.execOutsLastclock[name] = cd.stopBlock
					break
				}
				if err != nil {
					return nil, err
				}
				cd.seenClocks[item.BlockNum] = &pbsubstreams.Clock{
					Number:    item.BlockNum,
					Id:        item.BlockId,
					Timestamp: item.Timestamp,
				}
				cd.execOutsLastclock[name] = item.BlockNum
			}
		}

		if cd.seenClocks[i] != nil {
			cd.nextClockNumber = i + 1
			return cd.seenClocks[i], nil
		}

	}
	return nil, io.EOF
}

// Iter returns an iterator that yields clocks from the distributor.
// This allows using range loops: for clock, err := range distributor.Iter(ctx) { ... }
func (cd *ClockDistributor) Iter(ctx context.Context) iter.Seq2[*pbsubstreams.Clock, error] {
	return func(yield func(*pbsubstreams.Clock, error) bool) {
		for {
			clock, err := cd.Next(ctx)
			if err == io.EOF {
				return
			}

			if !yield(clock, err) {
				return
			}
		}
	}
}
