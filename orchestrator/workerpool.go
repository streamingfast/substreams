package orchestrator

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/streamingfast/substreams"
	"github.com/streamingfast/substreams/block"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"go.uber.org/zap"
	"google.golang.org/grpc/metadata"
)

type WorkerPool struct {
	workers chan *Worker
}

func NewWorkerPool(workerCount int, originalRequestModules *pbsubstreams.Modules, grpcClientFactory substreams.GrpcClientFactory) *WorkerPool {
	zlog.Info("initiating worker pool", zap.Int("worker_count", workerCount))
	workers := make(chan *Worker, workerCount)
	for i := 0; i < workerCount; i++ {
		workers <- &Worker{
			originalRequestModules: originalRequestModules,
			grpcClientFactory:      grpcClientFactory,
		}
	}
	return &WorkerPool{
		workers: workers,
	}
}

func (p *WorkerPool) Borrow() *Worker {
	w := <-p.workers
	return w
}

func (p *WorkerPool) ReturnWorker(worker *Worker) {
	p.workers <- worker
}

type Worker struct {
	grpcClientFactory      substreams.GrpcClientFactory
	originalRequestModules *pbsubstreams.Modules
}

func (w *Worker) Run(ctx context.Context, job *Job, respFunc substreams.ResponseFunc) ([]*block.Range, error) {
	start := time.Now()

	jobLogger := zlog.With(zap.Object("job", job))
	grpcClient, connClose, grpcCallOpts, err := w.grpcClientFactory(ctx)
	if err != nil {
		jobLogger.Error("getting grpc client", zap.Error(err))
		return nil, fmt.Errorf("grpc client factory: %w", err)
	}
	defer connClose()

	ctx = metadata.NewOutgoingContext(ctx, metadata.New(map[string]string{"substreams-partial-mode": "true"}))

	request := job.createRequest(w.originalRequestModules)

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
	if hosts := meta.Get("hostname"); len(hosts) != 0 {
		remoteHostname = hosts[0]
		jobLogger = jobLogger.With(zap.String("remote_hostname", remoteHostname))
	}

	jobLogger.Info("running job", zap.Object("job", job))
	defer func() {
		jobLogger.Info("job completed", zap.Object("job", job), zap.Duration("in", time.Since(start)))
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

			for _, progress := range resp.GetProgress().Modules {
				if f := progress.GetFailed(); f != nil {
					return nil, fmt.Errorf("module %s failed on host: %s", progress.Name, f.Reason)
				}
			}

			if err != nil {
				jobLogger.Warn("worker done on respFunc error", zap.Error(err))
				return nil, fmt.Errorf("sending progress: %w", err)
			}
		case *pbsubstreams.Response_SnapshotData:
			_ = r.SnapshotData
		case *pbsubstreams.Response_SnapshotComplete:
			_ = r.SnapshotComplete
		case *pbsubstreams.Response_Data:
			// These are not returned by virtue of `returnOutputs`
		}
	}
}
