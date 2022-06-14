package orchestrator

import (
	"github.com/streamingfast/substreams/block"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"go.uber.org/zap/zapcore"
)

type Job struct {
	requestRange       *block.Range
	moduleName         string // target
	moduleSaveInterval uint64
}

func (j *Job) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	enc.AddString("module_name", j.moduleName)
	enc.AddUint64("module_save_interval", j.moduleSaveInterval)
	enc.AddUint64("start_block", j.requestRange.StartBlock)
	enc.AddUint64("end_block", j.requestRange.ExclusiveEndBlock)
	return nil
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
