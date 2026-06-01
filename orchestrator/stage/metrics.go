package stage

import (
	"time"

	"github.com/streamingfast/substreams/block"

	"go.uber.org/zap"
)

type mergeMetrics struct {
	start         time.Time
	getStoreStart time.Time
	getStoreEnd   time.Time
	loadStart     time.Time
	loadEnd       time.Time
	mergeStart    time.Time
	mergeEnd      time.Time
	saveStart     time.Time
	saveEnd       time.Time
	writeStart    time.Time
	writeEnd      time.Time

	blockRange *block.Range
	stage      int
	moduleName string
	moduleHash string
}

func (m mergeMetrics) logFields() []zap.Field {
	f := []zap.Field{
		zap.String("total_time", time.Since(m.start).String()),
	}
	if !m.getStoreStart.IsZero() {
		f = append(f, zap.String("get_store_time", m.getStoreEnd.Sub(m.getStoreStart).String()))
	}
	if !m.loadStart.IsZero() {
		f = append(f, zap.String("load_time", m.loadEnd.Sub(m.loadStart).String()))
	}

	if !m.mergeStart.IsZero() {
		f = append(f, zap.String("merge_time", m.mergeEnd.Sub(m.mergeStart).String()))
	}

	if !m.saveEnd.IsZero() {
		f = append(f, zap.String("save_time", m.saveEnd.Sub(m.saveStart).String()))
	}

	if !m.writeEnd.IsZero() {
		f = append(f, zap.String("write_time", m.writeEnd.Sub(m.writeStart).String()))
	}

	if m.blockRange != nil {
		f = append(f, zap.Uint64("start_block", m.blockRange.StartBlock), zap.Uint64("end_block", m.blockRange.ExclusiveEndBlock))
	}

	f = append(f, zap.Int("stage", m.stage), zap.String("module_name", m.moduleName), zap.String("module_hash", m.moduleHash))

	return f
}
