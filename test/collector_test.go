package integration

import (
	"context"

	"github.com/streamingfast/dmetering"
	"github.com/streamingfast/substreams"
	"github.com/streamingfast/substreams/metering"
	pbssinternal "github.com/streamingfast/substreams/pb/sf/substreams/intern/v2"
	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
	"github.com/streamingfast/substreams/reqctx"
	"google.golang.org/protobuf/proto"
)

type eventsCollector struct {
	events []dmetering.Event
}

func (c *eventsCollector) Emit(_ context.Context, ev dmetering.Event) {
	c.events = append(c.events, ev)
}

func (c *eventsCollector) Shutdown(_ error) {}

func (c *eventsCollector) Events() []dmetering.Event {
	return c.events
}

var eventsCollectorKey = "eventsCollector"

func withEventsCollector(ctx context.Context, collector *eventsCollector) context.Context {
	return context.WithValue(ctx, eventsCollectorKey, collector)
}

func eventsCollectorFromContext(ctx context.Context) *eventsCollector {
	if ev, ok := ctx.Value(eventsCollectorKey).(*eventsCollector); ok {
		return ev
	}
	return &eventsCollector{}
}

type responseCollector struct {
	*eventsCollector

	responses         []*pbsubstreamsrpc.Response
	internalResponses []*pbssinternal.ProcessRangeResponse

	sender *metering.MetricsSender

	outputModuleName string
	ctx              context.Context
	startBlock       uint64
	endBlock         uint64
}

func newResponseCollector(ctx context.Context, outputModuleName string, startBlock, endBlock uint64) *responseCollector {
	rc := &responseCollector{
		outputModuleName: outputModuleName,
		startBlock:       startBlock,
		endBlock:         endBlock,
	}
	rc.ctx = reqctx.WithEmitter(ctx, rc)
	rc.eventsCollector = eventsCollectorFromContext(ctx)
	rc.sender = metering.NewMetricsSender()

	return rc
}

func (c *responseCollector) Collect(respAny substreams.ResponseFromAnyTier) error {
	switch resp := respAny.(type) {
	case *pbsubstreamsrpc.Response:
		c.responses = append(c.responses, resp)
		metering.AddEgressBytes(c.ctx, proto.Size(resp))
		c.sender.Send(c.ctx, "test_org", "test_api_key", "10.0.0.1", "test_meta", "testOutputHash", "tier1")
	case *pbssinternal.ProcessRangeResponse:
		// in non-test code, this is 'passed through' from tier2 to tier1 to the user as a pbsubstreamsrpc.response
		if blockScopedData := resp.GetBlockScopedData(); blockScopedData != nil {
			if blockScopedData.Clock.Number < c.endBlock && blockScopedData.Clock.Number >= c.startBlock {
				c.responses = append(c.responses, &pbsubstreamsrpc.Response{
					Message: &pbsubstreamsrpc.Response_BlockScopedData{
						BlockScopedData: &pbsubstreamsrpc.BlockScopedData{
							Clock: blockScopedData.Clock,
							Output: &pbsubstreamsrpc.MapModuleOutput{
								Name:      c.outputModuleName,
								MapOutput: blockScopedData.Output,
							},
						},
					},
				})
			}
		}

		c.internalResponses = append(c.internalResponses, resp)
		c.sender.Send(c.ctx, "test_org", "test_api_key", "10.0.0.1", "test_meta", "testOutputHash", "tier2")
	}
	return nil
}
