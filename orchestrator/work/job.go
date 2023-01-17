package work

import (
	"fmt"
	"strings"

	"github.com/streamingfast/substreams/block"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"go.uber.org/zap/zapcore"
)

// Job is a single unit of scheduling, meaning it is a request that goes to a
// remote gRPC service for execution.
type Job struct {
	ModuleName   string // target
	RequestRange *block.Range
	// the order of the job, as a unit of job scheduling, relative to the position in the chain.
	requiredModules []string // modules that need to be sync'd before this one starts at RequestRange.StartBlockNum}
	priority        int
}

func NewJob(storeName string, requestRange *block.Range, requiredModules []string, priority int) *Job {
	// TODO(abourget): test that the priority calculations give us what we need
	// The thing is that the priority wouldn't change.. the readiness is what would
	// change really. That's handled in the Plan, but priority is constant.
	// We'll schedule them when we can
	j := &Job{
		ModuleName:      storeName,
		RequestRange:    requestRange,
		requiredModules: requiredModules,
		priority:        priority,
	}
	return j
}

func (j *Job) Matches(moduleName string, blockNum uint64) bool {
	return j.ModuleName == moduleName && j.RequestRange.Contains(blockNum)
}

func (j *Job) CreateRequest(originalModules *pbsubstreams.Modules) *pbsubstreams.Request {
	return &pbsubstreams.Request{
		StartBlockNum: int64(j.RequestRange.StartBlock),
		StopBlockNum:  j.RequestRange.ExclusiveEndBlock,
		ForkSteps:     []pbsubstreams.ForkStep{pbsubstreams.ForkStep_STEP_IRREVERSIBLE},
		//IrreversibilityCondition: irreversibilityCondition, // Unsupported for now
		Modules:      originalModules,
		OutputModule: j.ModuleName,
	}
}

func (j *Job) String() string {
	return fmt.Sprintf("job: module=%s range=%s deps=%s prio=%d", j.ModuleName, j.RequestRange, strings.Join(j.requiredModules, ","), j.priority)
}

func (j *Job) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	enc.AddString("module_name", j.ModuleName)
	enc.AddUint64("start_block", j.RequestRange.StartBlock)
	enc.AddUint64("end_block", j.RequestRange.ExclusiveEndBlock)
	//enc.AddArray("deps", j.deps)
	return nil
}
