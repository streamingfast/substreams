package work

import (
	"fmt"

	"github.com/streamingfast/substreams/block"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"go.uber.org/zap/zapcore"
)

// TODO(abourget): the Job shouldn't be the one prioritizing itself.. an external Scheduler would
// mutate the WorkPlan and reprioritize properly.

// Job is a single unit of scheduling, meaning it is a request that goes to a
// remote gRPC service for execution.
type Job struct {
	ModuleName   string // target
	RequestRange *block.Range
	Priority     int
	Scheduled    bool

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
		RequestRange: requestRange,
	}
	j.defineDependencies(ancestorStoreModules)
	j.Priority = len(j.deps)*2 + totalJobs - myJobIndex
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

func (j *Job) SignalDependencyResolved(storeName string, blockNum uint64) {
	for _, dep := range j.deps {
		if dep.storeName == storeName && blockNum >= j.RequestRange.StartBlock {
			dep.resolved = true
		}
	}
}

func (j *Job) ReadyForDispatch() bool {
	for _, dep := range j.deps {
		if !dep.resolved {
			return false
		}
	}
	return true
}

func (j *Job) CreateRequest(originalModules *pbsubstreams.Modules) *pbsubstreams.Request {
	return &pbsubstreams.Request{
		StartBlockNum: int64(j.RequestRange.StartBlock),
		StopBlockNum:  j.RequestRange.ExclusiveEndBlock,
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

type JobList []*Job

func (l JobList) MarshalLogArray(enc zapcore.ArrayEncoder) error {
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
	return fmt.Sprintf("job: module=%s range=%s", j.ModuleName, j.RequestRange)
}

func (j *Job) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	enc.AddString("module_name", j.ModuleName)
	enc.AddUint64("start_block", j.RequestRange.StartBlock)
	enc.AddUint64("end_block", j.RequestRange.ExclusiveEndBlock)
	//enc.AddArray("deps", j.deps)
	return nil
}
