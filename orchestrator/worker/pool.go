package worker

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/streamingfast/substreams"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

type Job struct {
	Request *pbsubstreams.Request
}

func (j *Job) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	enc.AddString("request_outputs", strings.Join(j.Request.OutputModules, "|"))
	enc.AddInt64("start_block", j.Request.StartBlockNum)
	enc.AddUint64("stop_block", j.Request.StopBlockNum)
	return nil
}

type Pool struct {
	workers chan *Worker
}

func NewPool(workerCount int, grpcClientFactory func() (pbsubstreams.StreamClient, []grpc.CallOption, error)) *Pool {
	zlog.Info("initiating worker pool", zap.Int("worker_count", workerCount))
	workers := make(chan *Worker, workerCount)
	for i := 0; i < workerCount; i++ {
		workers <- &Worker{
			grpcClientFactory: grpcClientFactory,
		}
	}
	return &Pool{
		workers: workers,
	}
}

func (p *Pool) Borrow() *Worker {
	w := <-p.workers
	return w
}

func (p *Pool) ReturnWorker(worker *Worker) {
	p.workers <- worker
}

type Worker struct {
	grpcClientFactory func() (pbsubstreams.StreamClient, []grpc.CallOption, error)
}

func (w *Worker) Run(ctx context.Context, job *Job, respFunc substreams.ResponseFunc) error {
	start := time.Now()
	zlog.Info("running job", zap.Object("job", job))
	defer func() {
		zlog.Info("job completed", zap.Object("job", job), zap.Duration("in", time.Since(start)))
	}()
	grpcClient, grpcCallOpts, err := w.grpcClientFactory()
	if err != nil {
		zlog.Error("getting grpc client", zap.Error(err))
		return err
	}
	reqCtx := metadata.NewOutgoingContext(ctx, metadata.New(map[string]string{"substreams-partial-mode": "true"}))
	stream, err := grpcClient.Blocks(reqCtx, job.Request, grpcCallOpts...)
	if err != nil {
		return fmt.Errorf("getting block stream: %w", err)
	}
	for {
		select {
		case <-ctx.Done():
			zlog.Warn("context cancel will waiting for stream data, worker is terminating")
			return ctx.Err()
		default:
		}

		resp, err := stream.Recv()
		if err != nil {
			if err == io.EOF {
				zlog.Info("worker done", zap.Object("job", job))
				return nil
			}
			zlog.Warn("worker done on stream error", zap.Error(err))
			return fmt.Errorf("receiving stream resp:%w", err)
		}

		switch r := resp.Message.(type) {
		case *pbsubstreams.Response_Progress:
			err := respFunc(resp)
			if err != nil {
				zlog.Warn("worker done on respFunc error", zap.Error(err))
				return fmt.Errorf("sending progress: %w", err)
			}
		case *pbsubstreams.Response_SnapshotData:
			_ = r.SnapshotData
		case *pbsubstreams.Response_SnapshotComplete:
			_ = r.SnapshotComplete
		case *pbsubstreams.Response_Data:
		}
	}
}
