package orchestrator

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBuildConfig(t *testing.T) {
	tests := []struct {
		storeInterval             int
		execOutInterval           int
		productionMode            bool
		graphInitBlock            uint64
		resolvedStartBlock        uint64
		linearHandoffBlock        uint64
		exclusiveEndBlock         uint64
		expectStoresRange         string
		expectMapWriteRange       string
		expectMapReadRange        string
		expectLinearPipelineRange string
	}{
		{
			100, 100,
			false, 621, 738, 738, 742,
			"621-742", "0-0", "0-0", "738-742",
		},
		{
			100, 100,
			false, 621, 738, 738, 842,
			"621-742", "0-0", "0-0", "738-842",
		},
		{
			100, 100,
			true, 621, 738, 742, 742,
			"621-700", "700-742", "738-742", "0-0",
		},
		{
			100, 100,
			true, 621, 738, 742, 842,
			"621-800", "700-842", "738-842", "0-0",
		},
		{
			100, 100,
			true, 621, 621, 942, 998,
			"621-942", "621-942", "621-942", "942-998",
		},
		{
			100, 100,
			true, 621, 621, 942, 0,
			"621-942", "621-942", "621-942", "942-999999",
		},
	}
	for i, tt := range tests {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			res := BuildConfig(tt.graphInitBlock, tt.resolvedStartBlock, tt.linearHandoffBlock, tt.exclusiveEndBlock, tt.productionMode)
			assert.Equal(t, tt.expectStoresRange, res.BuildStores.String())
			assert.Equal(t, tt.expectMapWriteRange, res.MapProduce.String())
			assert.Equal(t, tt.expectMapReadRange, res.MapRead.String())
			assert.Equal(t, tt.expectLinearPipelineRange, res.LinearPipeline.String())
		})
	}
}
