package service

import (
	"context"
	"fmt"

	"github.com/streamingfast/substreams"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"

	"google.golang.org/grpc/metadata"

	"github.com/streamingfast/substreams/service/config"
)

func TestNewService(runtimeConfig config.RuntimeConfig, linearHandoffBlockNum uint64, streamFactoryFunc StreamFactoryFunc) *Service {
	return &Service{
		blockType:          "sf.substreams.v1.test.Block",
		partialModeEnabled: true,
		streamFactoryFunc:  streamFactoryFunc,
		runtimeConfig:      runtimeConfig,
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

type nooptrailable struct{}

func (n nooptrailable) SetTrailer(md metadata.MD) {}

func (s *Service) TestBlocks(ctx context.Context, isSubRequest bool, request *pbsubstreams.Request, respFunc substreams.ResponseFunc) error {
	return s.blocks(ctx, s.runtimeConfig, isSubRequest, request, respFunc, &nooptrailable{})
}
