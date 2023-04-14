package test

import (
	"fmt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
	"os"
	"testing"
)

func Test_ReadSpecFromReader(t *testing.T) {
	file, err := os.Open("./testdata/test_spec.yaml")
	require.NoError(t, err)
	defer file.Close()

	spec, err := readSpecFromReader(file)
	require.NoError(t, err)

	assert.Equal(t, &Spec{
		Tests: []*TestConfig{
			{
				Module: "map_extract_data_types",
				Block:  12369910,
				Path:   ".feeGrowthGlobalUpdates[] | select(.poolAddress == '7858e59e0c01ea06df3af3d20ac7b0003275d4bf') | .newValue.value",
				Expect: "40709313040992720268568262802",
			},
		},
	}, spec)
}

func Test_WriteSpecFromReader(t *testing.T) {

	spec := &Spec{
		Tests: []*TestConfig{
			{
				Module: "map_extract_data_types",
				Block:  12369910,
				Path:   ".feeGrowthGlobalUpdates[] | select(.poolAddress == '7858e59e0c01ea06df3af3d20ac7b0003275d4bf') | .newValue.value",
				Expect: "40709313040992720268568262802",
			},
		},
	}

	out, err := yaml.Marshal(spec)
	require.NoError(t, err)
	fmt.Println(string(out))
}
