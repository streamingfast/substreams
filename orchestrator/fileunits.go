package orchestrator

// TODO(abourget): move to `work` under `orchestrator/work`
//

import (
	"fmt"
	"github.com/streamingfast/substreams/block"
)

type ModuleJobs []*Job
type JobUnit struct {
	partialsToGenerate block.Ranges
}

type File = block.Range
type PartialStoreFile File
type FullStoreFile File
type PartialStoreFiles = block.Ranges

// FileUnits contains all the file-related ranges of things we'll want to plan
// work for, and things that are already available.
type FileUnits struct {
	modName string

	initialCompleteRange *FullStoreFile // Points to a complete .kv file, to initialize the store upon getting started.
	partialsMissing      PartialStoreFiles
	partialsPresent      PartialStoreFiles
}

func NewFileUnits(modName string, storeSaveInterval, modInitBlock, workUpToBlockNum uint64, snapshots *Snapshots) (out *FileUnits, err error) {
	out = &FileUnits{modName: modName}
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
		out.initialCompleteRange = (*FullStoreFile)(block.NewRange(modInitBlock, completeSnapshot.ExclusiveEndBlock))

		if completeSnapshot.ExclusiveEndBlock == workUpToBlockNum {
			return
		}
	}

	for ptr := backProcessStartBlock; ptr < workUpToBlockNum; {
		end := minOf(ptr-ptr%storeSaveInterval+storeSaveInterval, workUpToBlockNum)
		newPartial := block.NewRange(ptr, end)
		if !snapshots.ContainsPartial(newPartial) {
			out.partialsMissing = append(out.partialsMissing, newPartial)
		} else {
			out.partialsPresent = append(out.partialsPresent, newPartial)
		}
		ptr = end
	}
	return
}

func (w *FileUnits) initialProcessedPartials() block.Ranges {
	return w.partialsPresent.Merged()
}

func (w *FileUnits) batchRequests(subreqSplitSize uint64) block.Ranges {
	return w.partialsMissing.MergedBuckets(subreqSplitSize)
}

func minOf(a, b uint64) uint64 {
	if a < b {
		return a
	}
	return b
}
