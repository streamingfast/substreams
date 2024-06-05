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
			name:                      "g1. dev mode with stop within same segment as start block",
			storeInterval:             100,
			productionMode:            false,
			needsStores:               true,
			graphInitBlock:            621,
			resolvedStartBlock:        738,
			linearHandoffBlock:        700,
			exclusiveEndBlock:         742,
			expectStoresRange:         "621-700",
			expectWriteExecOutRange:   "nil",
			expectReadExecOutRange:    "nil",
			expectLinearPipelineRange: "700-742",
		},
		{
			name:                      "g2. dev mode with stop in next segment",
			storeInterval:             100,
			productionMode:            false,
			needsStores:               true,
			graphInitBlock:            621,
			resolvedStartBlock:        738,
			linearHandoffBlock:        700,
			exclusiveEndBlock:         842,
			expectStoresRange:         "621-700",
			expectWriteExecOutRange:   "nil",
			expectReadExecOutRange:    "nil",
			expectLinearPipelineRange: "700-842",
		},
		{
			name:                      "g4. production within start and stop on the same segment",
			storeInterval:             100,
			productionMode:            true,
			needsStores:               true,
			graphInitBlock:            621,
			resolvedStartBlock:        738,
			linearHandoffBlock:        800,
			exclusiveEndBlock:         742,
			expectStoresRange:         "621-800",
			expectWriteExecOutRange:   "700-800",
			expectReadExecOutRange:    "738-742",
			expectLinearPipelineRange: "nil",
		},

		{
			name:                      "g5. production three different segments for init, start and stop block",
			storeInterval:             100,
			productionMode:            true,
			needsStores:               true,
			graphInitBlock:            621,
			resolvedStartBlock:        738,
			linearHandoffBlock:        900,
			exclusiveEndBlock:         842,
			expectStoresRange:         "621-900",
			expectWriteExecOutRange:   "700-900",
			expectReadExecOutRange:    "738-842",
			expectLinearPipelineRange: "nil",
		},

		{
			name:                      "g6. production, start is init, handoff as boundary, stop block in a next segment",
			storeInterval:             100,
			productionMode:            true,
			needsStores:               true,
			graphInitBlock:            621,
			resolvedStartBlock:        621,
			linearHandoffBlock:        900,
			exclusiveEndBlock:         998,
			expectStoresRange:         "621-900",
			expectWriteExecOutRange:   "621-900",
			expectReadExecOutRange:    "621-900",
			expectLinearPipelineRange: "900-998",
		},
		{
			name:                      "g7. production, start is init, handoff as boundary, stop block infinite",
			storeInterval:             100,
			productionMode:            true,
			needsStores:               true,
			graphInitBlock:            621,
			resolvedStartBlock:        621,
			linearHandoffBlock:        900,
			exclusiveEndBlock:         0,
			expectStoresRange:         "621-900",
			expectWriteExecOutRange:   "621-900",
			expectReadExecOutRange:    "621-900",
			expectLinearPipelineRange: "900-0",
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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := BuildTier1RequestPlan(tt.productionMode, uint64(tt.storeInterval), tt.graphInitBlock, tt.resolvedStartBlock, tt.linearHandoffBlock, tt.exclusiveEndBlock, tt.needsStores)
			assert.Nil(t, err)
			assert.Equal(t, tt.expectStoresRange, tostr(res.BuildStores), "buildStores")
			assert.Equal(t, tt.expectWriteExecOutRange, tostr(res.WriteExecOut), "writeExecOut")
			assert.Equal(t, tt.expectReadExecOutRange, tostr(res.ReadExecOut), "readExecOut")
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
