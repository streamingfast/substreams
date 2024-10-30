package integration

import (
	"context"

	"github.com/streamingfast/dmetering"
	"github.com/streamingfast/substreams"
	"github.com/streamingfast/substreams/metering"
	pbssinternal "github.com/streamingfast/substreams/pb/sf/substreams/intern/v2"
	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
	"github.com/streamingfast/substreams/reqctx"
)

type eventsCollector struct {
	events []dmetering.Event
}

func (c *eventsCollector) Emit(_ context.Context, ev dmetering.Event) {
	c.events = append(c.events, ev)
}

func (c *eventsCollector) Shutdown(_ error) {
	return
}

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

	ctx context.Context
}

func newResponseCollector(ctx context.Context) *responseCollector {
	rc := &responseCollector{}
	rc.ctx = reqctx.WithEmitter(ctx, rc)
	rc.eventsCollector = eventsCollectorFromContext(ctx)
	rc.sender = metering.NewMetricsSender()

	return rc
}

func (c *responseCollector) Collect(respAny substreams.ResponseFromAnyTier) error {
	switch resp := respAny.(type) {
	case *pbsubstreamsrpc.Response:
		c.responses = append(c.responses, resp)
		c.sender.Send(c.ctx, "test_user", "test_api_key", "10.0.0.1", "test_meta", "testOutputHash", "tier1", resp)
	case *pbssinternal.ProcessRangeResponse:
		c.internalResponses = append(c.internalResponses, resp)
		c.sender.Send(c.ctx, "test_user", "test_api_key", "10.0.0.1", "test_meta", "testOutputHash", "tier2", resp)
	}
	return nil
}
