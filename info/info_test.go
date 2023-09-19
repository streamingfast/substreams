package info

import (
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestBasicInfo(t *testing.T) {
	info, err := Basic("https://github.com/streamingfast/substreams-uniswap-v3/releases/download/v0.2.8/substreams.spkg")
	require.NoError(t, err)

	r, err := json.MarshalIndent(info, "", "  ")
	require.NoError(t, err)

	fmt.Println(string(r))
}

func TestExtendedInfo(t *testing.T) {
	info, err := Extended("https://github.com/streamingfast/substreams-uniswap-v3/releases/download/v0.2.8/substreams.spkg", "graph_out")
	require.NoError(t, err)

	r, err := json.MarshalIndent(info, "", "  ")
	require.NoError(t, err)

	fmt.Println(string(r))
}
