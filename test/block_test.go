package pipeline

import (
	"github.com/streamingfast/bstream"
	"github.com/streamingfast/dstore"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	pbsubstreamstest "github.com/streamingfast/substreams/pb/sf/substreams/v1/test"
	"github.com/streamingfast/substreams/pipeline"
	"github.com/streamingfast/substreams/store"
	"strconv"
)

type blockProcessedCallBack func(p *pipeline.Pipeline, b *bstream.Block, stores store.Map, baseStore dstore.Store)

type TestBlockGenerator interface {
	Generate() []*pbsubstreamstest.Block
}

type LinearBlockGenerator struct {
	startBlock         uint64
	inclusiveStopBlock uint64
}

func (g LinearBlockGenerator) Generate() []*pbsubstreamstest.Block {
	var blocks []*pbsubstreamstest.Block
	for i := g.startBlock; i <= g.inclusiveStopBlock; i++ {
		blocks = append(blocks, &pbsubstreamstest.Block{
			Id:     "block-" + strconv.FormatUint(i, 10),
			Number: i,
			Step:   int32(bstream.StepNewIrreversible),
		})
	}
	return blocks
}

type responseCollector struct {
	responses []*pbsubstreams.Response
}

func newResponseCollector() *responseCollector {
	return &responseCollector{
		responses: []*pbsubstreams.Response{},
	}
}

func (c *responseCollector) Collect(resp *pbsubstreams.Response) error {
	c.responses = append(c.responses, resp)
	return nil
}

type NewTestBlockGenerator func(startBlock uint64, inclusiveStopBlock uint64) TestBlockGenerator
