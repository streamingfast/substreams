package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"go.uber.org/zap/zapcore"

	"github.com/streamingfast/substreams"
	"github.com/streamingfast/substreams/block"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"go.uber.org/zap"
	"google.golang.org/grpc/metadata"
)

type WorkerPool struct {
	workers  chan *Worker
	JobStats map[*Job]*JobStat
}

type JobStat struct {
	ModuleName string
	StartAt    time.Time

	RequestRange *block.Range

	CurrentBlock   uint64
	RemainingBlock uint64
	BlockCount     uint64
	BlockSec       float64

	RemoteHost string
}

type WorkerStats struct {
	JobStats       []*JobStat
	CountPerModule map[string]uint64
}

func (j *JobStat) update(currentBlock uint64) {
	j.CurrentBlock = currentBlock
	j.RemainingBlock = j.RequestRange.ExclusiveEndBlock - j.CurrentBlock
	j.BlockSec = float64(j.BlockCount) / time.Since(j.StartAt).Seconds()
	j.BlockCount = j.CurrentBlock - j.RequestRange.StartBlock
}

func (j *JobStat) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	enc.AddString("remote_host", j.RemoteHost)
	enc.AddString("module_name", j.ModuleName)
	err := enc.AddObject("request_range", j.RequestRange)
	if err != nil {
		return err
	}
	enc.AddTime("start_at", j.StartAt)
	enc.AddUint64("current_block", j.CurrentBlock)
	enc.AddUint64("block_count", j.BlockCount)
	enc.AddUint64("remaining_blocks", j.RemainingBlock)
	enc.AddFloat64("block_secs", float64(j.BlockCount)/time.Since(j.StartAt).Seconds())
	return nil
}

func NewWorkerPool(workerCount int, grpcClientFactory substreams.GrpcClientFactory) *WorkerPool {
	zlog.Info("initiating worker pool", zap.Int("worker_count", workerCount))

	workers := make(chan *Worker, workerCount)
	for i := 0; i < workerCount; i++ {
		workers <- &Worker{
			grpcClientFactory: grpcClientFactory,
		}
	}

	workerPool := &WorkerPool{
		workers:  workers,
		JobStats: map[*Job]*JobStat{},
	}

	go func() {
		for {
			select {
			case <-time.After(time.Second * 5):
				var jobStats []*JobStat
				countPerModule := map[string]uint64{}
				zlog.Info("work pool job stats", zap.Int("job_count", len(workerPool.JobStats)))
				for _, value := range workerPool.JobStats {
					if count, ok := countPerModule[value.ModuleName]; ok {
						count++
						countPerModule[value.ModuleName] = count
					} else {
						countPerModule[value.ModuleName] = 1
					}
					jobStats = append(jobStats, value)
				}

				workerStats := &WorkerStats{
					JobStats:       jobStats,
					CountPerModule: countPerModule,
				}

				content, err := json.Marshal(workerStats)
				if err != nil {
					zlog.Warn("failed to marshall job statistics", zap.Error(err))
				}
				fmt.Println("worker statistics", string(content))
			}
		}
	}()
	return workerPool
}

func (p *WorkerPool) Borrow() *Worker {
	w := <-p.workers
	return w
}

func (p *WorkerPool) ReturnWorker(worker *Worker) {
	p.workers <- worker
}

type Worker struct {
	grpcClientFactory substreams.GrpcClientFactory
}

func (w *Worker) Run(ctx context.Context, job *Job, jobStats map[*Job]*JobStat, requestModules *pbsubstreams.Modules, respFunc substreams.ResponseFunc) ([]*block.Range, error) {
	start := time.Now()

	jobLogger := zlog.With(zap.Object("job", job))
	grpcClient, connClose, grpcCallOpts, err := w.grpcClientFactory(ctx)
	if err != nil {
		jobLogger.Error("getting grpc client", zap.Error(err))
		return nil, fmt.Errorf("grpc client factory: %w", err)
	}
	defer connClose()

	ctx = metadata.NewOutgoingContext(ctx, metadata.New(map[string]string{"substreams-partial-mode": "true"}))

	request := job.createRequest(requestModules)

	stream, err := grpcClient.Blocks(ctx, request, grpcCallOpts...)
	if err != nil {
		jobLogger.Error("getting block stream", zap.Error(err))
		return nil, fmt.Errorf("getting block stream: %w", err)
	}
	defer func() {
		stream.CloseSend()
	}()

	meta, err := stream.Header()
	if err != nil {
		jobLogger.Warn("error getting stream header", zap.Error(err))
	}
	remoteHostname := "unknown"
	if hosts := meta.Get("host"); len(hosts) != 0 {
		remoteHostname = hosts[0]
		jobLogger = jobLogger.With(zap.String("remote_hostname", remoteHostname))
	}

	jobStat := &JobStat{
		ModuleName:   job.moduleName,
		RequestRange: job.requestRange,
		StartAt:      time.Now(),
		CurrentBlock: job.requestRange.StartBlock,
		RemoteHost:   remoteHostname,
	}
	jobStats[job] = jobStat

	jobLogger.Info("running job", zap.Object("job", job))
	defer func() {
		jobLogger.Info("job completed", zap.Object("job", job), zap.Duration("in", time.Since(start)), zap.Object("job_stat", jobStat))
		delete(jobStats, job)
	}()

	for {
		select {
		case <-ctx.Done():
			jobLogger.Warn("context cancel will waiting for stream data, worker is terminating")
			return nil, ctx.Err()
		default:
		}

		resp, err := stream.Recv()
		if err != nil {
			if err == io.EOF {
				jobLogger.Info("worker done", zap.Object("job", job))
				trailers := stream.Trailer().Get("substreams-partials-written")
				var partialsWritten []*block.Range
				if len(trailers) != 0 {
					jobLogger.Info("partial written", zap.String("trailer", trailers[0]))
					partialsWritten = block.ParseRanges(trailers[0])
				}
				return partialsWritten, nil
			}
			jobLogger.Warn("worker done on stream error", zap.Error(err))
			return nil, fmt.Errorf("receiving stream resp: %w", err)
		}

		switch r := resp.Message.(type) {
		case *pbsubstreams.Response_Progress:
			err := respFunc(resp)
			if err != nil {
				jobLogger.Warn("worker done on respFunc error", zap.Error(err))
				return nil, fmt.Errorf("sending progress: %w", err)
			}
			// fixme: why is range nil?
			//jobStat.update(resp.GetProgress().Modules[0].GetProcessedRanges().ProcessedRanges[len(resp.GetProgress().Modules[0].GetProcessedRanges().ProcessedRanges)-1].EndBlock)
		case *pbsubstreams.Response_SnapshotData:
			_ = r.SnapshotData
		case *pbsubstreams.Response_SnapshotComplete:
			_ = r.SnapshotComplete
		case *pbsubstreams.Response_Data:
			// These are not returned by virtue of `returnOutputs`
		}
	}
}
