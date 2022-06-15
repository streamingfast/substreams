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
	workPlan WorkPlan,
	subreqSplit uint64,
	stores map[string]*state.Store,
	graph *manifest.ModuleGraph,
	pool *RequestPool,
) (*OrderedStrategy, error) {
	for storeName, store := range stores {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
			// do nothing
		}
		workUnit := workPlan[storeName]
		zlog.Debug("new ordered strategy", zap.String("builder", store.Name))

		//TODO(abourget): get the requests we want to submit here..

		requests := workUnit.batchRequests(subreqSplit)
		rangeLen := len(requests)
		for idx, requestRange := range requests {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			default:
				// do nothing
			}
			// TODO(abourget): here we loop WorkUnit.reqChunks, and grab the ancestor modules
			// to setup the waiter.
			// blockRange's start/end come from `requestRange`
			ancestorStoreModules, err := graph.AncestorStoresOf(store.Name)
			if err != nil {
				return nil, fmt.Errorf("getting ancestore stores for %s: %w", store.Name, err)
			}

			job := &Job{
				moduleName:         store.Name,
				moduleSaveInterval: store.SaveInterval,
				requestRange:       requestRange,
			}

			//req := createRequest(requestRange, store.Name, request.IrreversibilityCondition, request.Modules)
			waiter := NewWaiter(requestRange.StartBlock, ancestorStoreModules...)
			_ = pool.Add(ctx, rangeLen-idx, job, waiter)

			zlog.Info("request created", zap.String("module_name", store.Name), zap.Uint64("start_block", requestRange.StartBlock), zap.Uint64("end_block", requestRange.ExclusiveEndBlock))
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
			job, err := s.requestPool.GetNext(ctx)
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
