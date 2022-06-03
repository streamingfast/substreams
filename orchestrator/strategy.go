package orchestrator

import (
	"context"
	"fmt"
	"io"

	"github.com/streamingfast/substreams/manifest"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/state"
	"go.uber.org/zap"
)

type Strategy interface {
	GetNextRequest(ctx context.Context) (*Job, error)
}

type RequestGetter interface {
	Get(ctx context.Context) (*Job, error)
}

type OrderedStrategy struct {
	requestGetter RequestGetter
}

func NewOrderedStrategy(
	ctx context.Context,
	splitWorks SplitWorkModules,
	request *pbsubstreams.Request,
	stores map[string]*state.Store,
	graph *manifest.ModuleGraph,
	pool *RequestPool,
) (*OrderedStrategy, error) {
	for storeName, store := range stores {
		workUnit := splitWorks[storeName]
		zlog.Debug("new ordered strategy", zap.String("builder", store.Name))

		rangeLen := len(workUnit.reqChunks)
		for idx, reqChunk := range workUnit.reqChunks {
			// TODO(abourget): here we loop SplitWork.reqChunks, and grab the ancestor modules
			// to setup the waiter.
			// blockRange's start/end come from `reqChunk`
			ancestorStoreModules, err := graph.AncestorStoresOf(store.Name)
			if err != nil {
				return nil, fmt.Errorf("getting ancestore stores for %s: %w", store.Name, err)
			}

			job := &Job{
				moduleName: store.Name,
				reqChunk:   reqChunk,
			}

			//req := createRequest(reqChunk, store.Name, request.IrreversibilityCondition, request.Modules)
			waiter := NewWaiter(reqChunk.start, ancestorStoreModules...)
			_ = pool.Add(ctx, rangeLen-idx, job, waiter)

			zlog.Info("request created", zap.String("module_name", store.Name), zap.Uint64("start_block", reqChunk.start), zap.Uint64("end_block", reqChunk.end))
		}
	}

	pool.Start(ctx)

	return &OrderedStrategy{
		requestGetter: pool,
	}, nil
}

func (d *OrderedStrategy) GetNextRequest(ctx context.Context) (*Job, error) {
	req, err := d.requestGetter.Get(ctx)
	if err == io.EOF {
		return nil, io.EOF
	}
	if err != nil {
		return nil, err
	}

	return req, nil
}

func GetRequestStream(ctx context.Context, strategy Strategy) <-chan *Job {
	stream := make(chan *Job)

	go func() {
		defer close(stream)

		for {
			r, err := strategy.GetNextRequest(ctx)
			if err == io.EOF || err == context.DeadlineExceeded || err == context.Canceled {
				return
			}
			if err != nil {
				panic(err)
			}

			if r == nil {
				continue
			}

			select {
			case <-ctx.Done():
				return
			case stream <- r:
			}
		}
	}()

	return stream
}
