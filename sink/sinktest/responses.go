package sinktest

import (
	"strconv"
	"testing"
	"time"

	"github.com/streamingfast/bstream"
	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// DefaultOutputModuleName is the output module name of the responses [BlockScopedData] creates.
const DefaultOutputModuleName = "map_output"

// FinalBlock can be used in [BlockScopedData] and [BlockUndo] to pass final block for the response.
type FinalBlock string

// FixedBaseTime is the time of block #0, [BlockScopedData] gives block N a time N minutes later.
var FixedBaseTime = time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

// StreamMock is a helper function that creates a stream of responses for testing, see [NewFakeStreamServer].
func StreamMock(responses ...any) []any {
	return responses
}

// BlockScopedData creates a BlockScopedData response for block blockIdentifier (`10a` is block #10 with
// id `10a`) with output. extraArgs can be a [FinalBlock] or a *timestamppb.Timestamp for the block time.
func BlockScopedData(t testing.TB, blockIdentifier string, output proto.Message, extraArgs ...any) *pbsubstreamsrpc.Response {
	t.Helper()

	blockNum, blockId := ExpandBlockIdentifier(blockIdentifier)
	blockTime := timestamppb.New(FixedBaseTime.Add(time.Duration(blockNum * uint64(time.Minute))))

	outputData, err := anypb.New(output)
	require.NoError(t, err)

	currentRef := bstream.NewBlockRef(blockId, blockNum)
	finalRef := bstream.BlockRefEmpty

	for _, arg := range extraArgs {
		switch v := arg.(type) {
		case FinalBlock:
			finalBlockNum, finalBlockId := ExpandBlockIdentifier(string(v))
			finalRef = bstream.NewBlockRef(finalBlockId, finalBlockNum)

		case *timestamppb.Timestamp:
			blockTime = v
		}
	}

	cursor := bstream.Cursor{Step: bstream.StepNew, Block: currentRef, HeadBlock: currentRef, LIB: finalRef}
	if currentRef.ID() == finalRef.ID() {
		cursor.Step = bstream.StepNewIrreversible
	}

	return &pbsubstreamsrpc.Response{
		Message: &pbsubstreamsrpc.Response_BlockScopedData{
			BlockScopedData: &pbsubstreamsrpc.BlockScopedData{
				Cursor: cursor.ToOpaque(),
				Clock: &pbsubstreams.Clock{
					Id:        blockId,
					Number:    blockNum,
					Timestamp: blockTime,
				},
				Output: &pbsubstreamsrpc.MapModuleOutput{
					Name:      DefaultOutputModuleName,
					MapOutput: outputData,
				},
				FinalBlockHeight: finalRef.Num(),
			},
		},
	}
}

// BlockUndo creates a BlockUndoSignal response back to lastValidBlockIdentifier. extraArgs can be a
// [FinalBlock].
func BlockUndo(t testing.TB, lastValidBlockIdentifier string, extraArgs ...any) *pbsubstreamsrpc.Response {
	t.Helper()

	blockNum, blockId := ExpandBlockIdentifier(lastValidBlockIdentifier)
	finalRef := bstream.BlockRefEmpty

	for _, arg := range extraArgs {
		switch v := arg.(type) {
		case FinalBlock:
			finalBlockNum, finalBlockId := ExpandBlockIdentifier(string(v))
			finalRef = bstream.NewBlockRef(finalBlockId, finalBlockNum)
		}
	}

	cursor := bstream.Cursor{Step: bstream.StepUndo, Block: bstream.NewBlockRef(blockId, blockNum), HeadBlock: bstream.NewBlockRef(blockId, blockNum), LIB: finalRef}

	return &pbsubstreamsrpc.Response{
		Message: &pbsubstreamsrpc.Response_BlockUndoSignal{
			BlockUndoSignal: &pbsubstreamsrpc.BlockUndoSignal{
				LastValidBlock:  &pbsubstreams.BlockRef{Id: blockId, Number: blockNum},
				LastValidCursor: cursor.ToOpaque(),
			},
		},
	}
}

// BlockTime can be used in [BlockScopedData] to specify the block time for the response.
// Always returns time in UTC to match database output.
func BlockTime(t testing.TB, in string) time.Time {
	return BlockTimepb(t, in).AsTime().UTC()
}

// BlockTimepb can be used in [BlockScopedData] to specify the block time for the response.
func BlockTimepb(t testing.TB, in string) *timestamppb.Timestamp {
	t.Helper()

	if in == "now" {
		return timestamppb.Now()
	}

	if parsedTime, err := time.Parse(time.RFC3339, in); err == nil {
		return timestamppb.New(parsedTime)
	}

	if parsedTime, err := time.Parse("2006-01-02 15:04:05", in); err == nil {
		return timestamppb.New(parsedTime)
	}

	if parsedTime, err := time.Parse("2006-01-02", in); err == nil {
		return timestamppb.New(parsedTime)
	}

	require.Fail(t, "invalid block time format", "expected RFC3339, <2006-01-02 15:04:05> or <2006-01-02> formats, got %q", in)
	return nil // This line will never be reached due to require.Fail
}

// ExpandBlockIdentifier splits a block identifier like `10a` into its number (10) and its id (`10a`).
func ExpandBlockIdentifier(in string) (blockNum uint64, blockId string) {
	blockId = in
	if in == "" {
		return 0, "0"
	}

	i := 0
	for i < len(in) && in[i] >= '0' && in[i] <= '9' {
		i++
	}

	if i > 0 {
		// Parse the numeric part
		if num, err := strconv.ParseUint(in[:i], 10, 64); err == nil {
			blockNum = num
		}
	}

	return
}
