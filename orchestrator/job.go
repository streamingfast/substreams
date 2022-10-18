package orchestrator

import (
	"fmt"

	"github.com/streamingfast/substreams/block"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"go.uber.org/zap/zapcore"
)


// TODO(abourget): the Job shouldn't be the one prioritizing itself.. an external Scheduler would
// mutate the WorkPlan and reprioritize properly.
type Job struct {
	ModuleName   string // target
	requestRange *block.Range
	priority     int
	scheduled    bool

	deps jobDependencies
}

/*

P:   9 8 7
1:   A B C

P:   10 9 8
2:   D  E F

P:   11 10 9
3:   G  H  I

*/

func NewJob(storeName string, requestRange *block.Range, ancestorStoreModules []*pbsubstreams.Module, totalJobs, myJobIndex int) *Job {
	j := &Job{
		ModuleName:   storeName,
		requestRange: requestRange,
	}
	j.defineDependencies(ancestorStoreModules)
	j.priority = len(j.deps) * 2 + totalJobs - myJobIndex
	return j
}

func (j *Job) defineDependencies(stores []*pbsubstreams.Module) {
	for _, store := range stores {
		// Here dependencies do not highlight any block range, so if we have
		// ranges that are already completed, we'll have suboptimal planning.
		j.deps = append(j.deps, &jobDependency{
			storeName: store.Name,
			resolved:  false,
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

func (j *Job) CreateRequest(originalModules *pbsubstreams.Modules) *pbsubstreams.Request {
	return &pbsubstreams.Request{
		StartBlockNum: int64(j.requestRange.StartBlock),
		StopBlockNum:  j.requestRange.ExclusiveEndBlock,
		ForkSteps:     []pbsubstreams.ForkStep{pbsubstreams.ForkStep_STEP_IRREVERSIBLE},
		//IrreversibilityCondition: irreversibilityCondition, // Unsupported for now
		Modules:       originalModules,
		OutputModules: []string{j.ModuleName},
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
	return fmt.Sprintf("job: module=%s range=%s", j.ModuleName, j.requestRange)
}

func (j *Job) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	enc.AddString("module_name", j.ModuleName)
	enc.AddUint64("start_block", j.requestRange.StartBlock)
	enc.AddUint64("end_block", j.requestRange.ExclusiveEndBlock)
	//enc.AddArray("deps", j.deps)
	return nil
}
