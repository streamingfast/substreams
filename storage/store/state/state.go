package state

import (
	"fmt"

	"github.com/streamingfast/substreams/utils"

	"github.com/streamingfast/substreams/block"
	"go.uber.org/zap/zapcore"
)

// ModuleStorageState contains all the file-related ranges of store snapshots
// we'll want to plan work for, and things that are already available.
type StoreStorageState struct {
	ModuleName         string
	ModuleInitialBlock uint64

	InitialCompleteRange *FullStoreFile // Points to a complete .kv file, to initialize the store upon getting started.
	PartialsMissing      PartialStoreFiles
	PartialsPresent      PartialStoreFiles
}

type FullStoreFile = block.Range
type PartialStoreFiles = block.Ranges

func NewStoreStorageState(modName string, storeSaveInterval, modInitBlock, workUpToBlockNum uint64, snapshots *storeSnapshots) (out *StoreStorageState, err error) {
	out = &StoreStorageState{ModuleName: modName, ModuleInitialBlock: modInitBlock}
	if workUpToBlockNum <= modInitBlock {
		return
	}

	completeSnapshot := snapshots.LastCompleteSnapshotBefore(workUpToBlockNum)
	if completeSnapshot != nil && completeSnapshot.ExclusiveEndBlock <= modInitBlock {
		return nil, fmt.Errorf("cannot have saved last store before module's init block")
	}

	parallelProcessStartBlock := modInitBlock
	if completeSnapshot != nil {
		parallelProcessStartBlock = completeSnapshot.ExclusiveEndBlock
		out.InitialCompleteRange = (*FullStoreFile)(block.NewRange(modInitBlock, completeSnapshot.ExclusiveEndBlock))

		if completeSnapshot.ExclusiveEndBlock == workUpToBlockNum {
			return
		}
	}

	for ptr := parallelProcessStartBlock; ptr < workUpToBlockNum; {
		end := utils.MinOf(ptr-ptr%storeSaveInterval+storeSaveInterval, workUpToBlockNum)
		newPartial := block.NewRange(ptr, end)
		if !snapshots.ContainsPartial(newPartial) {
			out.PartialsMissing = append(out.PartialsMissing, newPartial)
		} else {
			out.PartialsPresent = append(out.PartialsPresent, newPartial)
		}
		ptr = end
	}
	return
}

func (s *StoreStorageState) Name() string { return s.ModuleName }

func (s *StoreStorageState) BatchRequests(subreqSplitSize uint64) block.Ranges {
	return s.PartialsMissing.MergedBuckets(subreqSplitSize)
}

func (s *StoreStorageState) InitialProgressRanges() (out block.Ranges) {
	if s.InitialCompleteRange != nil {
		out = append(out, s.InitialCompleteRange)
	}
	out = append(out, s.PartialsPresent.Merged()...)
	return
}
func (s *StoreStorageState) ReadyUpToBlock() uint64 {
	if s.InitialCompleteRange == nil {
		return s.ModuleInitialBlock
	}
	return s.InitialCompleteRange.ExclusiveEndBlock
}

func (w *StoreStorageState) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	enc.AddString("store_name", w.ModuleName)
	enc.AddString("intial_range", w.InitialCompleteRange.String())
	enc.AddInt("partial_missing", len(w.PartialsMissing))
	enc.AddInt("partial_present", len(w.PartialsPresent))
	return nil
}
