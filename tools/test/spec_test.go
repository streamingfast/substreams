package test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_readSpecFromFile(t *testing.T) {
	tests := []struct {
		filepath string
		expect   *Spec
	}{
		{
			filepath: "./testdata/test_spec.yaml",
			expect: &Spec{
				Tests: []*TestConfig{
					{
						Module: "map_extract_data_types",
						Block:  12369910,
						Path:   `.feeGrowthGlobalUpdates[] | select(.poolAddress == "7858e59e0c01ea06df3af3d20ac7b0003275d4bf") | .newValue.value`,
						Expect: "40709313040992720268568262802",
					},
					{
						Module: "map_extract_data_types",
						Block:  12369910,
						Path:   ".foo",
						Op:     "float",
						Expect: "2.5",
					},
					{
						Module: "map_extract_data_types",
						Block:  12369910,
						Path:   `.feeGrowthGlobalUpdates[] | select(.poolAddress == "6c6bc977e13df9b0de53b251522280bb72383700") | .newValue.value`,
						Expect: "329334915253227784464544",
					},
					{
						Module: "map_extract_data_types",
						Block:  12369910,
						Path:   `.transactions[] | select(.id == "06e53c0e241686b10a7e3aa5d3af706144a486d291e2036489ed0c4b62f75cdd") | .gasUsed`,
						Op:     "float",
						Args:   "error=2",
						Expect: "217278",
					},
					{
						Module: "store_pool_liquidities",
						Block:  12370014,
						Path:   `select(.key == "liquidity:c2e9f25be6257c210d7adf0d4cd6e3e881ba25f8") | .new`,
						Expect: "222633640125805970242",
					},
					{
						Module: "store_pools",
						Block:  12370078,
						Path:   `select(.key == "pool:6f48eca74b38d2936b02ab603ff4e36a6c0e3a77") | .new.token1.totalSupply`,
						Expect: "25916147047969262",
					},
				},
			},
		},
		{
			filepath: "./testdata/test_spec.jsonl",
			expect: &Spec{
				Tests: []*TestConfig{
					{
						Module: "module_name",
						Block:  16371052,
						Path:   ".feeGrowthGlobalUpdates[] | select(.poolAddress == \"0x6c6bc977e13df9b0de53b251522280bb72383700\") | .newValue.value",
						Expect: "10223378799466963208063206201",
					},
					{
						Module: "module_name",
						Block:  16371053,
						Path:   ".feeGrowthGlobalUpdates[] | select(.poolAddress == \"0x6c6bc977e13df9b0de53b251522280bb72383700\") | .newValue.value",
						Expect: "10223378799466963208063206201",
					},
				},
			},
		},
		{
			filepath: "./testdata/test_spec.csv",
			expect: &Spec{
				Tests: []*TestConfig{
					{
						Module: "module_name",
						Block:  16371052,
						Path:   ".feeGrowthGlobalUpdates[] | select(.poolAddress == \"0x6c6bc977e13df9b0de53b251522280bb72383700\") | .newValue.value",
						Expect: "10223378799466963208063206201",
					},
					{
						Module: "module_name",
						Block:  16371053,
						Path:   ".feeGrowthGlobalUpdates[] | select(.poolAddress == \"0x6c6bc977e13df9b0de53b251522280bb72383700\") | .newValue.value",
						Expect: "10223378799466963208063206201",
					},
					{
						Module: "module_name",
						Block:  16371053,
						Path:   ".feeGrowthGlobalUpdates[]",
						Expect: "10223378799466963208063206201",
					},
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.filepath, func(t *testing.T) {
			spec, err := readSpecFromFile(test.filepath)
			require.NoError(t, err)
			assert.Equal(t, test.expect, spec)
		})
	}
}
