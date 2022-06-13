package orchestrator

import (
	"context"
	"fmt"
	"io"

	"github.com/streamingfast/substreams/manifest"
	"github.com/streamingfast/substreams/state"
	"go.uber.org/zap"
)

type OrderedStrategy struct {
	requestPool *RequestPool
}

func NewOrderedStrategy(
	ctx context.Context,
	splitWorks SplitWorkModules,
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
				moduleName:         store.Name,
				moduleSaveInterval: store.SaveInterval,
				reqChunk:           reqChunk,
			}

			//req := createRequest(reqChunk, store.Name, request.IrreversibilityCondition, request.Modules)
			waiter := NewWaiter(reqChunk.start, ancestorStoreModules...)
			_ = pool.Add(ctx, rangeLen-idx, job, waiter)

			zlog.Info("request created", zap.String("module_name", store.Name), zap.Uint64("start_block", reqChunk.start), zap.Uint64("end_block", reqChunk.end))
		}
	}

	pool.Start(ctx)

	return &OrderedStrategy{
		requestPool: pool,
	}, nil
}

func (s *OrderedStrategy) getRequestStream(ctx context.Context) <-chan *Job {
	requestsStream := make(chan *Job)
	go func() {
		defer close(requestsStream)

		for {
			job, err := s.requestPool.getNext(ctx)
			if err == io.EOF {
				zlog.Debug("EOF in getRequestStream")
				return
			}
			select {
			case <-ctx.Done():
				zlog.Debug("ctx cannnlaskdfjlkj")
				return
			case requestsStream <- job:
			}
		}
	}()
	return requestsStream
}
