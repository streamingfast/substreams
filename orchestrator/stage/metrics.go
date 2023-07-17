package stage

import (
	"time"

	"go.uber.org/zap"
)

type mergeMetrics struct {
	start      time.Time
	loadStart  time.Time
	loadEnd    time.Time
	mergeStart time.Time
	mergeEnd   time.Time
	saveStart  time.Time
	saveEnd    time.Time
}

func (m mergeMetrics) logFields() []zap.Field {
	f := []zap.Field{
		zap.String("total_time", time.Since(m.start).String()),
		zap.String("load_time", m.loadEnd.Sub(m.loadStart).String()),
		zap.String("merge_time", m.mergeEnd.Sub(m.mergeStart).String()),
	}
	if !m.saveEnd.IsZero() {
		f = append(f, zap.String("save_time", m.saveEnd.Sub(m.saveStart).String()))
	}
	return f
}
