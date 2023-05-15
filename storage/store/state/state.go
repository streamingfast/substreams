package state

import (
	"fmt"

	"github.com/streamingfast/substreams/block"
	"github.com/streamingfast/substreams/storage/store"
	"github.com/streamingfast/substreams/utils"
	"go.uber.org/zap/zapcore"
)

// ModuleStorageState contains all the file-related ranges of store snapshots
// we'll want to plan work for, and things that are already available.
type StoreStorageState struct {
	ModuleName         string
	ModuleInitialBlock uint64

	InitialCompleteFile *store.FileInfo // Points to a complete .kv file, to initialize the store upon getting started.
	PartialsMissing     block.Ranges
	PartialsPresent     store.FileInfos
}

func NewStoreStorageState(modName string, storeSaveInterval, modInitBlock, workUpToBlockNum uint64, snapshots *storeSnapshots) (out *StoreStorageState, err error) {
	out = &StoreStorageState{ModuleName: modName, ModuleInitialBlock: modInitBlock}
	if workUpToBlockNum <= modInitBlock {
		return
	}

	completeSnapshot := snapshots.LastCompleteSnapshotBefore(workUpToBlockNum)
	if completeSnapshot != nil && completeSnapshot.Range.ExclusiveEndBlock <= modInitBlock {
		return nil, fmt.Errorf("cannot have saved last store before module's init block")
	}

	parallelProcessStartBlock := modInitBlock
	if completeSnapshot != nil {
		parallelProcessStartBlock = completeSnapshot.Range.ExclusiveEndBlock
		out.InitialCompleteFile = completeSnapshot

		if completeSnapshot.Range.ExclusiveEndBlock == workUpToBlockNum {
			return
		}
	}

	for ptr := parallelProcessStartBlock; ptr < workUpToBlockNum; {
		end := utils.MinOf(ptr-ptr%storeSaveInterval+storeSaveInterval, workUpToBlockNum)
		newPartial := block.NewRange(ptr, end)
		if partialFile := snapshots.findPartial(newPartial); partialFile == nil {
			out.PartialsMissing = append(out.PartialsMissing, newPartial)
		} else {
			out.PartialsPresent = append(out.PartialsPresent, partialFile)
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
	if s.InitialCompleteFile != nil {
		out = append(out, s.InitialCompleteFile.Range)
	}
	out = append(out, s.PartialsPresent.Ranges().Merged()...)
	return
}
func (s *StoreStorageState) ReadyUpToBlock() uint64 {
	if s.InitialCompleteFile == nil {
		return s.ModuleInitialBlock
	}
	return s.InitialCompleteFile.Range.ExclusiveEndBlock
}

func (w *StoreStorageState) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	enc.AddString("store_name", w.ModuleName)
	bRange := "None"
	if w.InitialCompleteFile != nil {
		bRange = w.InitialCompleteFile.Range.String()
	}
	enc.AddString("intial_range", bRange)
	enc.AddInt("partial_missing", len(w.PartialsMissing))
	enc.AddInt("partial_present", len(w.PartialsPresent))
	return nil
}
