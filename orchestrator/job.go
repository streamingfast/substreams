package orchestrator

import (
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"go.uber.org/zap/zapcore"
)

type Job struct {
	reqChunk   *reqChunk
	moduleName string // target
	//	Request    *pbsubstreams.Request
}

func (j *Job) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	enc.AddString("module_name", j.moduleName)
	enc.AddUint64("start_block", j.reqChunk.start)
	enc.AddUint64("end_block", j.reqChunk.end)
	return nil
}

func (job *Job) createRequest(originalModules *pbsubstreams.Modules) *pbsubstreams.Request {
	return &pbsubstreams.Request{
		StartBlockNum: int64(job.reqChunk.start),
		StopBlockNum:  job.reqChunk.end,
		ForkSteps:     []pbsubstreams.ForkStep{pbsubstreams.ForkStep_STEP_IRREVERSIBLE},
		//IrreversibilityCondition: irreversibilityCondition, // Unsupported for now
		Modules:       originalModules,
		OutputModules: []string{job.moduleName},
	}
}
