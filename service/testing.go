package service

import (
	"context"
	"fmt"

	"github.com/streamingfast/substreams"
	pbssinternal "github.com/streamingfast/substreams/pb/sf/substreams/intern/v2"
	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
	"github.com/streamingfast/substreams/service/config"
	"github.com/streamingfast/substreams/storage/store"
)

// TestTraceID must be used everywhere a TraceID is required. It must be the same
// between tier1 and tier2, otherwise tier1 will not find the file produced by
// tier2 correctly.
var TestTraceID = "00000000000000000000000000000000"

func TestTraceIDParam() store.TraceIDParam {
	return store.TraceIDParam(TestTraceID)
}

func TestNewService(runtimeConfig config.RuntimeConfig, linearHandoffBlockNum uint64, streamFactoryFunc StreamFactoryFunc) *Tier1Service {
	return &Tier1Service{
		blockType:         "sf.substreams.v1.test.Block",
		streamFactoryFunc: streamFactoryFunc,
		runtimeConfig:     runtimeConfig,
		getRecentFinalBlock: func() (uint64, error) {
			if linearHandoffBlockNum != 0 {
				return linearHandoffBlockNum, nil
			}
			return 0, fmt.Errorf("no live feed")
		},
		tracer: nil,
		logger: zlog,
	}
}

func (s *Tier1Service) TestBlocks(ctx context.Context, isSubRequest bool, request *pbsubstreamsrpc.Request, respFunc substreams.ResponseFunc) error {
	return s.blocks(ctx, request, respFunc)
}

func TestNewServiceTier2(runtimeConfig config.RuntimeConfig, streamFactoryFunc StreamFactoryFunc) *Tier2Service {
	return &Tier2Service{
		blockType:         "sf.substreams.v1.test.Block",
		streamFactoryFunc: streamFactoryFunc,
		runtimeConfig:     runtimeConfig,
		tracer:            nil,
		logger:            zlog,
	}
}

func (s *Tier2Service) TestBlocks(ctx context.Context, request *pbssinternal.ProcessRangeRequest, respFunc substreams.ResponseFunc, traceID *string) error {
	if traceID == nil {
		traceID = &TestTraceID
	}

	return s.processRange(ctx, request, respFunc, *traceID)
}
