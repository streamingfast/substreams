package integration

import (
	"github.com/streamingfast/substreams"
	pbssinternal "github.com/streamingfast/substreams/pb/sf/substreams/intern/v2"
	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
)

type responseCollector struct {
	responses         []*pbsubstreamsrpc.Response
	internalResponses []*pbssinternal.ProcessRangeResponse
}

func newResponseCollector() *responseCollector {
	return &responseCollector{}
}

func (c *responseCollector) Collect(respAny substreams.ResponseFromAnyTier) error {
	switch resp := respAny.(type) {
	case *pbsubstreamsrpc.Response:
		c.responses = append(c.responses, resp)
	case *pbssinternal.ProcessRangeResponse:
		c.internalResponses = append(c.internalResponses, resp)
	}
	return nil
}
