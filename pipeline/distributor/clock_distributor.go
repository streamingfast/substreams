package distributor

import (
	"context"
	"io"
	"iter"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/pipeline/exec"
	"github.com/streamingfast/substreams/storage/execout"
	execp "github.com/streamingfast/substreams/storage/path"
)

type ClockDistributor struct {
	execOuts                 map[string]execout.FileReader
	execOutsLastclock        map[string]uint64
	seenClocks               map[uint64]*pbsubstreams.Clock
	startBlock               uint64
	stopBlock                uint64
	nextClockNumber          uint64
	outputModuleInputs       []*pbsubstreams.Module_Input
	stagedModuleExecutorsMap map[string]*execp.ExecutorPath
	stagedModuleExecutors    [][]exec.ModuleExecutor
}

func NewClockDistributor(execOuts map[string]execout.FileReader, startBlock uint64, stopBlock uint64, outputModuleInputs []*pbsubstreams.Module_Input, stagedModuleExecutorsMap map[string]*execp.ExecutorPath, stagedModuleExecutors [][]exec.ModuleExecutor) *ClockDistributor {
	return &ClockDistributor{
		execOuts:                 execOuts,
		execOutsLastclock:        make(map[string]uint64),
		seenClocks:               make(map[uint64]*pbsubstreams.Clock),
		startBlock:               startBlock,
		stopBlock:                stopBlock,
		nextClockNumber:          startBlock,
		outputModuleInputs:       outputModuleInputs,
		stagedModuleExecutorsMap: stagedModuleExecutorsMap,
		stagedModuleExecutors:    stagedModuleExecutors,
	}
}

func (cd *ClockDistributor) Next(ctx context.Context) (*pbsubstreams.Clock, error) {
	for i := cd.nextClockNumber; i < cd.stopBlock; i++ {
		indicesSkip := true
		if len(cd.stagedModuleExecutorsMap) > 0 {
			for _, input := range cd.outputModuleInputs {

				if source := input.GetSource(); source != nil {
					indicesSkip = false
					break
				}

				if store := input.GetStore(); store != nil {
					//Cannot skip if store is deltas
					if store.GetMode() == pbsubstreams.Module_Input_Store_DELTAS {
						indicesSkip = false
						break
					}
				}

				if m := input.GetMap(); m != nil {
					executerPath := cd.stagedModuleExecutorsMap[m.ModuleName]
					executer := cd.stagedModuleExecutors[executerPath.LayerIndex][executerPath.ModuleIndex]

					if blockIndex := executer.BlockIndex(); blockIndex != nil {
						if !blockIndex.Skip(i) {
							indicesSkip = false
						}
					} else {
						//If on module as no index, we cannot skip the block
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
