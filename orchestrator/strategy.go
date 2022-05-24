package orchestrator

import (
	"context"
	"fmt"
	"io"

	"github.com/streamingfast/substreams/block"
	"github.com/streamingfast/substreams/manifest"
	"io"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/state"
	"go.uber.org/zap"
)

type Strategy interface {
	GetNextRequest(ctx context.Context) (*pbsubstreams.Request, error)
	RequestCount() int
}

type LinearStrategy struct {
	requests []*pbsubstreams.Request
}

func (s *LinearStrategy) RequestCount() int {
	return len(s.requests)
}

func NewLinearStrategy(ctx context.Context, request *pbsubstreams.Request, builders []*state.Builder, upToBlockNum uint64, blockRangeSizeSubRequests int) (*LinearStrategy, error) {
	res := &LinearStrategy{}

	for _, builder := range builders {
		zlog.Debug("squashables", zap.String("builder", builder.Name))
		zlog.Debug("up to block num", zap.Uint64("up_to_block_num", upToBlockNum))
		if upToBlockNum == builder.ModuleStartBlock {
			continue // nothing to synchronize
		}

		endBlock := upToBlockNum
		info, err := builder.Info(ctx)
		if err != nil {
			return nil, fmt.Errorf("getting builder info: %w", err)
		}

		lastExclusiveEndBlock := info.LastKVSavedBlock
		zlog.Info("got info", zap.Object("builder", builder), zap.Uint64("up_to_block", upToBlockNum), zap.Uint64("end_block", lastExclusiveEndBlock))
		if upToBlockNum <= lastExclusiveEndBlock {
			zlog.Debug("no request created", zap.Uint64("up_to_block", upToBlockNum), zap.Uint64("last_exclusive_end_block", lastExclusiveEndBlock))
			continue // not sure if we should pop here
		}

		reqStartBlock := lastExclusiveEndBlock
		if reqStartBlock == 0 {
			reqStartBlock = builder.ModuleStartBlock
		}

		moduleFullRangeToProcess := &block.Range{
			StartBlock:        reqStartBlock,
			ExclusiveEndBlock: endBlock,
		}

		requestRanges := moduleFullRangeToProcess.Split(uint64(blockRangeSizeSubRequests))
		for _, r := range requestRanges {
			req := createRequest(r.StartBlock, r.ExclusiveEndBlock, builder.Name, request.ForkSteps, request.IrreversibilityCondition, request.Modules)
			res.requests = append(res.requests, req)
			zlog.Info("request created", zap.String("module_name", builder.Name), zap.Object("block_range", r))
		}
	}

	return res, nil
}

func (s *LinearStrategy) GetNextRequest(ctx context.Context) (*pbsubstreams.Request, error) {
	if len(s.requests) == 0 {
		return nil, io.EOF
	}

	var request *pbsubstreams.Request
	request, s.requests = s.requests[0], s.requests[1:]

	return request, nil
}

type RequestGetter interface {
	Get(ctx context.Context) (*pbsubstreams.Request, error)
}

type OrderedStrategy struct {
	requestGetter RequestGetter
}

func NewOrderedStrategy(ctx context.Context, request *pbsubstreams.Request, builders []*state.Builder, graph *manifest.ModuleGraph, pool *Pool, upToBlockNum uint64) (*OrderedStrategy, error) {
	for _, builder := range builders {
		zlog.Debug("squashables", zap.String("builder", builder.Name))
		zlog.Debug("up to block num", zap.Uint64("up_to_block_num", upToBlockNum))
		if upToBlockNum == builder.ModuleStartBlock {
			continue // nothing to synchronize
		}

		endBlock := upToBlockNum
		info, err := builder.Info(ctx)
		if err != nil {
			return nil, fmt.Errorf("getting builder info: %w", err)
		}

		lastExclusiveEndBlock := info.LastKVSavedBlock
		zlog.Debug("got info", zap.Object("builder", builder), zap.Uint64("up_to_block", upToBlockNum), zap.Uint64("end_block", lastExclusiveEndBlock))
		if upToBlockNum <= lastExclusiveEndBlock {
			zlog.Debug("no request created", zap.Uint64("up_to_block", upToBlockNum), zap.Uint64("last_exclusive_end_block", lastExclusiveEndBlock))
			continue // not sure if we should pop here
		}

		reqStartBlock := lastExclusiveEndBlock
		if reqStartBlock == 0 {
			reqStartBlock = builder.ModuleStartBlock
		}

		requestBlockRange := &block.Range{
			StartBlock:        reqStartBlock,
			ExclusiveEndBlock: endBlock,
		}

		blockRanges := requestBlockRange.Split(200)
		for _, blockRange := range blockRanges {
			ancestorStoreModules, err := graph.AncestorStoresOf(builder.Name)
			if err != nil {
				return nil, fmt.Errorf("getting ancestore stores for %s: %w", builder.Name, err)
			}

			request := createRequest(blockRange.StartBlock, blockRange.ExclusiveEndBlock, builder.Name, request.ForkSteps, request.IrreversibilityCondition, request.Modules)
			waiter := NewWaiter(blockRange.StartBlock, ancestorStoreModules...)
			_ = pool.Add(ctx, request, waiter)

			zlog.Info("request created", zap.String("module_name", builder.Name), zap.Object("block_range", blockRange))
		}
	}

	return &OrderedStrategy{
		requestGetter: pool,
	}, nil
}

func (d *OrderedStrategy) GetNextRequest(ctx context.Context) (*pbsubstreams.Request, error) {
	req, err := d.requestGetter.Get(ctx)
	if err == io.EOF {
		return nil, io.EOF
	}
	if err != nil {
		return nil, err
	}

	return req, nil
}

func GetRequestStream(ctx context.Context, strategy Strategy) <-chan *pbsubstreams.Request {
	stream := make(chan *pbsubstreams.Request)

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
				//
			}
		}
	}()

	return stream
}

func createRequest(
	startBlock, stopBlock uint64,
	outputModuleName string,
	forkSteps []pbsubstreams.ForkStep,
	irreversibilityCondition string,
	modules *pbsubstreams.Modules,
) *pbsubstreams.Request {
	return &pbsubstreams.Request{
		StartBlockNum:            int64(startBlock),
		StopBlockNum:             stopBlock,
		ForkSteps:                forkSteps,
		IrreversibilityCondition: irreversibilityCondition,
		Modules:                  modules,
		OutputModules:            []string{outputModuleName},
	}
}
