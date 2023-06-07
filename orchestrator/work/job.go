package work

import (
	"fmt"

	"go.uber.org/zap/zapcore"

	"github.com/streamingfast/substreams/block"
	pbssinternal "github.com/streamingfast/substreams/pb/sf/substreams/intern/v2"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

// Job is a single unit of scheduling, meaning it is a request that goes to a
// remote gRPC service for execution.
type Job struct {
	//ModuleName   string // target
	RequestRange *block.Range
	Stage        int

	OutputModule string // should be the same top-level module as the on in the request
}

func NewJob(requestRange *block.Range, stage int, outputModule string) *Job {
	j := &Job{
		OutputModule: outputModule,
		RequestRange: requestRange,
		Stage:        stage,
	}
	return j
}

func (j *Job) CreateRequest(originalModules *pbsubstreams.Modules) *pbssinternal.ProcessRangeRequest {
	return &pbssinternal.ProcessRangeRequest{
		StartBlockNum: j.RequestRange.StartBlock,
		StopBlockNum:  j.RequestRange.ExclusiveEndBlock,
		Modules:       originalModules,
		OutputModule:  j.OutputModule,
		Stage:         uint32(j.Stage),
	}
}

func (j *Job) String() string {
	return fmt.Sprintf("job: stage=%d range=%s", j.Stage, j.RequestRange)
}

func (j *Job) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	enc.AddInt("stage", j.Stage)
	enc.AddUint64("start_block", j.RequestRange.StartBlock)
	enc.AddUint64("end_block", j.RequestRange.ExclusiveEndBlock)
	return nil
}
