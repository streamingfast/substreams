package plan

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/streamingfast/substreams/block"
)

func TestBuildConfig(t *testing.T) {
	tests := []struct {
		name                      string
		storeInterval             int
		execOutInterval           int
		productionMode            bool
		graphInitBlock            uint64
		resolvedStartBlock        uint64
		linearHandoffBlock        uint64
		exclusiveEndBlock         uint64
		expectStoresRange         string
		expectExecOutRange        string
		expectLinearPipelineRange string
	}{
		{
			"g1. dev mode with stop within same segment as start block",
			100, 100,
			false, 621, 738, 738, 742,
			"621-738", "nil", "738-742",
		},
		{
			"g2. dev mode with stop in next segment",
			100, 100,
			false, 621, 738, 738, 842,
			"621-738", "nil", "738-842",
		},
		{
			"g3. production with handoff and stop within same segment",
			100, 100,
			true, 621, 738, 742, 742,
			"621-700", "700-742", "nil",
		},
		{
			"similar to g3. production with handoff on boundary",
			100, 100,
			true, 621, 738, 800, 800,
			"621-700", "700-800", "nil",
		},
		{
			"production, handoff 10k and start/init is 0, stop infinity (0)",
			100, 100,
			true, 0, 0, 10000, 0,
			"0-10000", "0-10000", "10000-0",
		},
		{
			"g4. production with handoff and stop in next segment",
			100, 100,
			true, 621, 738, 842, 842,
			"621-800", "700-842", "nil",
		},
		{
			"g5. production, start is init, start handoff and stop in three segments",
			100, 100,
			true, 621, 621, 942, 998,
			"621-942", "621-942", "942-998",
		},
		{
			"g6. production, start is init, start and handoff in two segments, stop infinity",
			100, 100,
			true, 621, 621, 942, 0,
			"621-942", "621-942", "942-0",
		},
		// This panics because we don't accept a start block prior to the graph init block.
		// Maybe we can unblock that in the future, but it's not really useful.
		// A fronting layer could have the start block be equal to the graph init block
		// since _nothing_ would be produced prior to the graph init block anyway.
		// And that _might already be the case_.
		//{
		//	"g7. production, start block is prior to graph init block",
		//	100, 100,
		//	true, 700, 621, 842, 842,
		//	"700-800", "700-842", "nil",
		//},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := BuildRequestPlan(tt.productionMode, uint64(tt.storeInterval), tt.graphInitBlock, tt.resolvedStartBlock, tt.linearHandoffBlock, tt.exclusiveEndBlock)
			assert.Equal(t, tt.expectStoresRange, tostr(res.BuildStores), "buildStores")
			assert.Equal(t, tt.expectExecOutRange, tostr(res.WriteExecOut), "writeExecOut")
			assert.Equal(t, tt.expectLinearPipelineRange, tostr(res.LinearPipeline), "linearPipeline")
		})
	}
}

func tostr(s *block.Range) string {
	if s == nil {
		return "nil"
	}
	return fmt.Sprintf("%d-%d", s.StartBlock, s.ExclusiveEndBlock)
}
