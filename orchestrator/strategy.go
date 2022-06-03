package orchestrator

import (
	"context"
	"fmt"
	"io"

	"github.com/streamingfast/substreams/block"
	"github.com/streamingfast/substreams/manifest"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/state"
	"go.uber.org/zap"
)

type Strategy interface {
	GetNextRequest(ctx context.Context) (*pbsubstreams.Request, error)
}

type RequestGetter interface {
	Get(ctx context.Context) (*pbsubstreams.Request, error)
}

type OrderedStrategy struct {
	requestGetter RequestGetter
}

func NewOrderedStrategy(
	ctx context.Context,
	storageState *StorageState,
	request *pbsubstreams.Request,
	stores map[string]*state.Store,
	graph *manifest.ModuleGraph,
	pool *RequestPool,
	upToBlockNum uint64,
	storeSaveInterval uint64,
	blockRangeSizeSubRequests int,
	maxRangeSize uint64,
) (*OrderedStrategy, error) {
	for _, store := range stores {
		zlog.Debug("new ordered strategy", zap.String("builder", store.Name), zap.Uint64("up_to_block_num", upToBlockNum))

		// TODO(abourget): this logic is to be replaced by obedience to the SplitWork
		if upToBlockNum == store.ModuleInitialBlock {
			zlog.Debug("nothing to sync")
			continue // nothing to synchronize
		}

		storeLastBlock := storageState.lastBlocks[store.Name]
		subreqStartBlock := computeStoreExclusiveEndBlock(storeLastBlock, upToBlockNum, storeSaveInterval, store.ModuleInitialBlock)
		if subreqStartBlock == upToBlockNum {
			zlog.Debug("already produced up to start block", zap.Uint64("up_to_block", upToBlockNum))
			continue
		}
		if subreqStartBlock == 0 {
			subreqStartBlock = store.ModuleInitialBlock
		}

		// TODO(abourget): this was done in `splitWork` already
		moduleFullRangeToProcess := &block.Range{
			StartBlock:        subreqStartBlock,
			ExclusiveEndBlock: upToBlockNum,
		}

		// if moduleFullRangeToProcess.Size() > maxRangeSize {
		// 	return nil, fmt.Errorf("subrequest size too big. request must be started closer to the head block. store %s is %d blocks from head (max is %d)", store.Name, moduleFullRangeToProcess.Size(), maxRangeSize)
		// }

		requestRanges := moduleFullRangeToProcess.Split(uint64(blockRangeSizeSubRequests))
		rangeLen := len(requestRanges)
		for idx, blockRange := range requestRanges {
			// TODO(abourget): here we loop SplitWork.reqChunks, and grab the ancestor modules
			// to setup the waiter.
			// blockRange's start/end come from `reqChunk`
			ancestorStoreModules, err := graph.AncestorStoresOf(store.Name)
			if err != nil {
				return nil, fmt.Errorf("getting ancestore stores for %s: %w", store.Name, err)
			}

			req := createRequest(blockRange.StartBlock, blockRange.ExclusiveEndBlock, store.Name, request.IrreversibilityCondition, request.Modules)
			waiter := NewWaiter(blockRange.StartBlock, storageState, ancestorStoreModules...)
			_ = pool.Add(ctx, rangeLen-idx, req, waiter)

			zlog.Info("request created", zap.String("module_name", store.Name), zap.Object("block_range", blockRange))
		}
	}

	pool.Start(ctx)

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
			}
		}
	}()

	return stream
}

func createRequest(
	startBlock, stopBlock uint64,
	outputModuleName string,
	irreversibilityCondition string,
	modules *pbsubstreams.Modules,
) *pbsubstreams.Request {
	return &pbsubstreams.Request{
		StartBlockNum: int64(startBlock),
		StopBlockNum:  stopBlock,
		ForkSteps:     []pbsubstreams.ForkStep{pbsubstreams.ForkStep_STEP_IRREVERSIBLE},
		//IrreversibilityCondition: irreversibilityCondition, // Unsupported for now
		Modules:       modules,
		OutputModules: []string{outputModuleName},
	}
}
