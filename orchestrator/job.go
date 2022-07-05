package orchestrator

import (
	"fmt"

	"github.com/streamingfast/substreams/block"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"go.uber.org/zap/zapcore"
)

type Job struct {
	requestRange       *block.Range
	moduleName         string // target
	moduleSaveInterval uint64
	priority           int
	scheduled          bool

	deps jobDependencies
}

func NewJob(storeName string, saveInterval uint64, requestRange *block.Range, ancestorStoreModules []*pbsubstreams.Module, totalJobs, myJobIndex int) *Job {
	j := &Job{
		moduleName:         storeName,
		moduleSaveInterval: saveInterval,
		requestRange:       requestRange,
	}
	j.defineDependencies(ancestorStoreModules)
	j.priority = len(j.deps) + totalJobs - myJobIndex
	return j
}

func (j *Job) defineDependencies(stores []*pbsubstreams.Module) {
	blockNum := j.requestRange.StartBlock
	for _, store := range stores {
		if blockNum <= store.InitialBlock {
			continue
		}

		j.deps = append(j.deps, &jobDependency{
			storeName: store.Name,
		})
	}
}

func (j *Job) signalDependencyResolved(storeName string, blockNum uint64) {
	for _, dep := range j.deps {
		if dep.storeName == storeName && blockNum >= j.requestRange.StartBlock {
			dep.resolved = true
		}
	}
}

func (j *Job) readyForDispatch() bool {
	for _, dep := range j.deps {
		if !dep.resolved {
			return false
		}
	}
	return true
}

func (j *Job) createRequest(originalModules *pbsubstreams.Modules) *pbsubstreams.Request {
	return &pbsubstreams.Request{
		StartBlockNum: int64(j.requestRange.StartBlock),
		StopBlockNum:  j.requestRange.ExclusiveEndBlock,
		ForkSteps:     []pbsubstreams.ForkStep{pbsubstreams.ForkStep_STEP_IRREVERSIBLE},
		//IrreversibilityCondition: irreversibilityCondition, // Unsupported for now
		Modules:       originalModules,
		OutputModules: []string{j.moduleName},
	}
}

type jobDependency struct {
	storeName string
	resolved  bool
}

type jobList []*Job

func (l jobList) MarshalLogArray(enc zapcore.ArrayEncoder) error {
	for _, d := range l {
		enc.AppendObject(d)
	}
	return nil
}

type jobDependencies []*jobDependency

func (l jobDependencies) MarshalLogArray(enc zapcore.ArrayEncoder) error {
	for _, d := range l {
		enc.AppendObject(d)
	}
	return nil
}

func (d *jobDependency) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	enc.AddString("store", d.storeName)
	enc.AddBool("resolved", d.resolved)
	return nil
}

func (j *Job) String() string {
	return fmt.Sprintf("job: module=%s range=%s", j.moduleName, j.requestRange)
}

func (j *Job) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	enc.AddString("module_name", j.moduleName)
	enc.AddUint64("module_save_interval", j.moduleSaveInterval)
	enc.AddUint64("start_block", j.requestRange.StartBlock)
	enc.AddUint64("end_block", j.requestRange.ExclusiveEndBlock)
	enc.AddArray("deps", j.deps)
	return nil
}
