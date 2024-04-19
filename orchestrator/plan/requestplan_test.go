package plan

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/streamingfast/substreams/block"
)

func TestBuildConfig(t *testing.T) {

	type testStruct struct {
		name                      string
		storeInterval             int
		productionMode            bool
		needsStores               bool
		graphInitBlock            uint64
		resolvedStartBlock        uint64
		linearHandoffBlock        uint64
		exclusiveEndBlock         uint64
		expectStoresRange         string
		expectWriteExecOutRange   string
		expectReadExecOutRange    string
		expectLinearPipelineRange string
	}

	tests := []testStruct{
		{
			name:                      "no parallel work to do prod mode",
			storeInterval:             100,
			productionMode:            false,
			needsStores:               true,
			graphInitBlock:            621,
			resolvedStartBlock:        621,
			linearHandoffBlock:        621,
			exclusiveEndBlock:         742,
			expectStoresRange:         "nil",
			expectWriteExecOutRange:   "nil",
			expectReadExecOutRange:    "nil",
			expectLinearPipelineRange: "621-742",
		},
		{
			name:                      "no parallel work to do dev mode",
			storeInterval:             100,
			productionMode:            true,
			needsStores:               true,
			graphInitBlock:            621,
			resolvedStartBlock:        621,
			linearHandoffBlock:        621,
			exclusiveEndBlock:         742,
			expectStoresRange:         "nil",
			expectWriteExecOutRange:   "nil",
			expectReadExecOutRange:    "nil",
			expectLinearPipelineRange: "621-742",
		},
		{
			name:                      "g1. dev mode with stop within same segment as start block",
			storeInterval:             100,
			productionMode:            false,
			needsStores:               true,
			graphInitBlock:            621,
			resolvedStartBlock:        738,
			linearHandoffBlock:        738,
			exclusiveEndBlock:         742,
			expectStoresRange:         "621-738",
			expectWriteExecOutRange:   "nil",
			expectReadExecOutRange:    "nil",
			expectLinearPipelineRange: "738-742",
		},
		{
			name:                      "g2. dev mode with stop in next segment",
			storeInterval:             100,
			productionMode:            false,
			needsStores:               true,
			graphInitBlock:            621,
			resolvedStartBlock:        738,
			linearHandoffBlock:        738,
			exclusiveEndBlock:         842,
			expectStoresRange:         "621-738",
			expectWriteExecOutRange:   "nil",
			expectReadExecOutRange:    "nil",
			expectLinearPipelineRange: "738-842",
		},
		{
			name:                      "g3. production with handoff and stop within same segment",
			storeInterval:             100,
			productionMode:            true,
			needsStores:               true,
			graphInitBlock:            621,
			resolvedStartBlock:        738,
			linearHandoffBlock:        742,
			exclusiveEndBlock:         742,
			expectStoresRange:         "621-742",
			expectWriteExecOutRange:   "700-742",
			expectReadExecOutRange:    "738-742",
			expectLinearPipelineRange: "nil",
		},
		{
			name:                      "similar to g3. production with handoff on boundary",
			storeInterval:             100,
			productionMode:            true,
			needsStores:               true,
			graphInitBlock:            621,
			resolvedStartBlock:        738,
			linearHandoffBlock:        800,
			exclusiveEndBlock:         800,
			expectStoresRange:         "621-800",
			expectWriteExecOutRange:   "700-800",
			expectReadExecOutRange:    "738-800",
			expectLinearPipelineRange: "nil",
		},
		{
			name:                      "production, handoff 10k and start/init is 0, stop infinity (0)",
			storeInterval:             100,
			productionMode:            true,
			needsStores:               true,
			graphInitBlock:            0,
			resolvedStartBlock:        0,
			linearHandoffBlock:        10000,
			exclusiveEndBlock:         0,
			expectStoresRange:         "0-10000",
			expectWriteExecOutRange:   "0-10000",
			expectReadExecOutRange:    "0-10000",
			expectLinearPipelineRange: "10000-0",
		},
		{
			name:                      "g4. production with handoff and stop in next segment",
			storeInterval:             100,
			productionMode:            true,
			needsStores:               true,
			graphInitBlock:            621,
			resolvedStartBlock:        738,
			linearHandoffBlock:        842,
			exclusiveEndBlock:         842,
			expectStoresRange:         "621-842",
			expectWriteExecOutRange:   "700-842",
			expectReadExecOutRange:    "738-842",
			expectLinearPipelineRange: "nil",
		},
		{
			name:                      "g5. production, start is init, start handoff and stop in three segments",
			storeInterval:             100,
			productionMode:            true,
			needsStores:               true,
			graphInitBlock:            621,
			resolvedStartBlock:        621,
			linearHandoffBlock:        942,
			exclusiveEndBlock:         998,
			expectStoresRange:         "621-942",
			expectWriteExecOutRange:   "621-942",
			expectReadExecOutRange:    "621-942",
			expectLinearPipelineRange: "942-998",
		},
		{
			name:                      "g6. production, start is init, start and handoff in two segments, stop infinity",
			storeInterval:             100,
			productionMode:            true,
			needsStores:               true,
			graphInitBlock:            621,
			resolvedStartBlock:        621,
			linearHandoffBlock:        942,
			exclusiveEndBlock:         0,
			expectStoresRange:         "621-942",
			expectWriteExecOutRange:   "621-942",
			expectReadExecOutRange:    "621-942",
			expectLinearPipelineRange: "942-0",
		},
		{
			name:                      "small segment, production",
			storeInterval:             1000,
			productionMode:            true,
			needsStores:               true,
			graphInitBlock:            5,
			resolvedStartBlock:        10,
			linearHandoffBlock:        20,
			exclusiveEndBlock:         20,
			expectStoresRange:         "5-20",
			expectWriteExecOutRange:   "5-20",
			expectReadExecOutRange:    "10-20",
			expectLinearPipelineRange: "nil",
		},
		{
			name:                      "dev, no store",
			storeInterval:             100,
			productionMode:            false,
			needsStores:               false,
			graphInitBlock:            5,
			resolvedStartBlock:        105,
			linearHandoffBlock:        105,
			exclusiveEndBlock:         0,
			expectStoresRange:         "nil",
			expectWriteExecOutRange:   "nil",
			expectReadExecOutRange:    "nil",
			expectLinearPipelineRange: "105-0",
		},
		{
			name:                      "prod, no store",
			storeInterval:             100,
			productionMode:            true,
			needsStores:               false,
			graphInitBlock:            5,
			resolvedStartBlock:        105,
			linearHandoffBlock:        300,
			exclusiveEndBlock:         0,
			expectStoresRange:         "nil",
			expectWriteExecOutRange:   "100-300",
			expectReadExecOutRange:    "105-300",
			expectLinearPipelineRange: "300-0",
		},
		{
			name:                      "req in live segment development",
			storeInterval:             10,
			productionMode:            true,
			needsStores:               false,
			graphInitBlock:            5,
			resolvedStartBlock:        105,
			linearHandoffBlock:        100,
			exclusiveEndBlock:         0,
			expectStoresRange:         "nil",
			expectWriteExecOutRange:   "nil",
			expectReadExecOutRange:    "nil",
			expectLinearPipelineRange: "100-0",
		},
		{
			name:                      "req in live segment development",
			storeInterval:             10,
			productionMode:            false,
			needsStores:               false,
			graphInitBlock:            5,
			resolvedStartBlock:        105,
			linearHandoffBlock:        100,
			exclusiveEndBlock:         0,
			expectStoresRange:         "nil",
			expectWriteExecOutRange:   "nil",
			expectReadExecOutRange:    "nil",
			expectLinearPipelineRange: "100-0",
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
			res, err := BuildTier1RequestPlan(tt.productionMode, uint64(tt.storeInterval), tt.graphInitBlock, tt.resolvedStartBlock, tt.linearHandoffBlock, tt.exclusiveEndBlock, tt.needsStores)
			assert.Nil(t, err)
			assert.Equal(t, tt.expectStoresRange, tostr(res.BuildStores), "buildStores")
			assert.Equal(t, tt.expectWriteExecOutRange, tostr(res.WriteExecOut), "writeExecOut")
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
