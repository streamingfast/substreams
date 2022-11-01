package pipeline

import (
	"fmt"
	"github.com/streamingfast/bstream"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/reqctx"
	grpccodes "google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func BuildRequestDetails(request *pbsubstreams.Request, isSubRequest bool) (*reqctx.RequestDetails, error) {
	effectiveStartBlock, err := resolveStartBlockNum(request)
	if err != nil {
		return nil, err
	}

	outMap := map[string]bool{}
	for _, modName := range request.OutputModules {
		outMap[modName] = true
	}

	return &reqctx.RequestDetails{
		Request:                request,
		EffectiveStartBlockNum: effectiveStartBlock,
		IsSubRequest:           isSubRequest,
		IsOutputModule:         outMap,
	}, nil
}

func resolveStartBlockNum(req *pbsubstreams.Request) (uint64, error) {
	// Should already be validated but we play safe here
	if req.StartBlockNum < 0 {
		return 0, status.Error(grpccodes.InvalidArgument, "start block num must be positive")
	}

	if req.StartCursor == "" {
		return uint64(req.StartBlockNum), nil
	}

	cursor, err := bstream.CursorFromOpaque(req.StartCursor)
	if err != nil {
		return 0, status.Errorf(grpccodes.InvalidArgument, "invalid start cursor %q: %s", cursor, err.Error())
	}

	if cursor.Step.Matches(bstream.StepNew) || cursor.Step.Matches(bstream.StepIrreversible) {
		return cursor.Block.Num() + 1, nil // this block was the last sent to the customer
	}
	if cursor.Step.Matches(bstream.StepUndo) {
		return cursor.Block.Num(), nil
	}
	return 0, fmt.Errorf("invalid start cursor step")
}
