package bundler

import (
	"time"

	"github.com/streamingfast/bstream"
	"github.com/streamingfast/dmetrics"
	"github.com/streamingfast/logging/zapx"
	"go.uber.org/zap"
)

type boundaryStats struct {
	creationStart time.Time

	boundaryProcessTime time.Duration
	procesingDataTime   time.Duration
	uploadedDuration    time.Duration

	totalBoundaryCount uint64
	boundary           *bstream.Range

	// averages
	avgUploadDuration          *dmetrics.AvgDurationCounter
	avgBoundaryProcessDuration *dmetrics.AvgDurationCounter
	avgDataProcessDuration     *dmetrics.AvgDurationCounter
}

func newStats() *boundaryStats {
	return &boundaryStats{
		avgUploadDuration:          dmetrics.NewAvgDurationCounter(30*time.Second, time.Second, "upload dur"),
		avgBoundaryProcessDuration: dmetrics.NewAvgDurationCounter(30*time.Second, time.Second, "boundary process dur"),
		avgDataProcessDuration:     dmetrics.NewAvgDurationCounter(30*time.Second, time.Second, "data process dur"),
	}
}

func (s *boundaryStats) startBoundary(b *bstream.Range) {
	s.creationStart = time.Now()
	s.boundary = b
	s.totalBoundaryCount++
	s.boundaryProcessTime = 0
	s.procesingDataTime = 0
	s.uploadedDuration = 0
}

func (s *boundaryStats) addUploadedDuration(dur time.Duration) {
	s.avgUploadDuration.AddDuration(dur)
	s.uploadedDuration = dur
}

func (s *boundaryStats) endBoundary() {
	dur := time.Since(s.creationStart)
	s.avgBoundaryProcessDuration.AddDuration(dur)
	s.boundaryProcessTime = dur
	s.avgDataProcessDuration.AddDuration(s.procesingDataTime)
}

func (s *boundaryStats) addProcessingDataDur(dur time.Duration) {
	s.procesingDataTime += dur
}

func (s *boundaryStats) Log() []zap.Field {
	return []zap.Field{
		zap.Uint64("file_count", s.totalBoundaryCount),
		zap.Stringer("boundary", s.boundary),
		zapx.HumanDuration("boundary_process_duration", s.boundaryProcessTime),
		zapx.HumanDuration("upload_duration", s.uploadedDuration),
		zapx.HumanDuration("data_process_duration", s.procesingDataTime),
		zapx.HumanDuration("avg_upload_duration", s.avgUploadDuration.Average()),
		zapx.HumanDuration("total_upload_duration", s.avgUploadDuration.Total()),
		zapx.HumanDuration("avg_boundary_process_duration", s.avgBoundaryProcessDuration.Average()),
		zapx.HumanDuration("total_boundary_process_duration", s.avgBoundaryProcessDuration.Total()),
		zapx.HumanDuration("avg_data_process_duration", s.avgDataProcessDuration.Average()),
		zapx.HumanDuration("total_data_process_duration", s.avgDataProcessDuration.Total()),
	}
}
