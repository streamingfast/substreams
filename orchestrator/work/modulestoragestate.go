package work

import (
	"fmt"
	"github.com/streamingfast/substreams/block"
	"go.uber.org/zap/zapcore"
)

type PartialStoreFile = block.Range
type FullStoreFile = block.Range
type PartialStoreFiles = block.Ranges

type ModuleStorageStateMap map[string]*ModuleStorageState

// ModuleStorageState contains all the file-related ranges of things we'll want to plan
// work for, and things that are already available.
type ModuleStorageState struct {
	ModuleName         string
	ModuleInitialBlock uint64

	InitialCompleteRange *FullStoreFile // Points to a complete .kv file, to initialize the store upon getting started.
	PartialsMissing      PartialStoreFiles
	PartialsPresent      PartialStoreFiles
}

func newModuleStorageState(modName string, storeSaveInterval, modInitBlock, workUpToBlockNum uint64, snapshots *Snapshots) (out *ModuleStorageState, err error) {
	out = &ModuleStorageState{ModuleName: modName, ModuleInitialBlock: modInitBlock}
	if workUpToBlockNum <= modInitBlock {
		return
	}

	completeSnapshot := snapshots.LastCompleteSnapshotBefore(workUpToBlockNum)
	if completeSnapshot != nil && completeSnapshot.ExclusiveEndBlock <= modInitBlock {
		return nil, fmt.Errorf("cannot have saved last store before module's init block")
	}

	backProcessStartBlock := modInitBlock
	if completeSnapshot != nil {
		backProcessStartBlock = completeSnapshot.ExclusiveEndBlock
		out.InitialCompleteRange = (*FullStoreFile)(block.NewRange(modInitBlock, completeSnapshot.ExclusiveEndBlock))

		if completeSnapshot.ExclusiveEndBlock == workUpToBlockNum {
			return
		}
	}

	for ptr := backProcessStartBlock; ptr < workUpToBlockNum; {
		end := minOf(ptr-ptr%storeSaveInterval+storeSaveInterval, workUpToBlockNum)
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

func (w *ModuleStorageState) initialProcessedPartials() block.Ranges {
	return w.PartialsPresent.Merged()
}

func (w *ModuleStorageState) batchRequests(subreqSplitSize uint64) block.Ranges {
	return w.PartialsMissing.MergedBuckets(subreqSplitSize)
}

func (w *ModuleStorageState) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	enc.AddString("store_name", w.ModuleName)
	enc.AddString("intial_range", w.InitialCompleteRange.String())
	enc.AddInt("partial_missing", len(w.PartialsMissing))
	enc.AddInt("partial_present", len(w.PartialsPresent))
	return nil
}

func minOf(a, b uint64) uint64 {
	if a < b {
		return a
	}
	return b
}
