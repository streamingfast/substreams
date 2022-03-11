package manifest

import (
	"encoding/base64"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestManifest_YamlUnmarshal(t *testing.T) {
	_, manifest, err := DecodeYamlManifestFromFile("./test/test_manifest.yaml")
	assert.NoError(t, err)
	assert.GreaterOrEqual(t, len(manifest.Modules), 1)
	assert.Equal(t, manifest.StartBlock, uint64(6809737))
}

func TestStreamYamlDecode(t *testing.T) {
	type test struct {
		name           string
		rawYamlInput   string
		expectedOutput Module
	}

	tests := []test{
		{
			name: "basic mapper",
			rawYamlInput: `---
name: pairExtractor
kind: map
code:
  file: ./pcs_substreams_bg.wasm.wasm
inputs:
  - source: proto:sf.ethereum.type.v1.Block
output:
  type: proto:pcs.types.v1.Pairs`,
			expectedOutput: Module{
				Name:   "pairExtractor",
				Kind:   "map",
				Code:   Code{File: "./pcs_substreams_bg.wasm.wasm"},
				Inputs: []*Input{{Source: "proto:sf.ethereum.type.v1.Block"}},
				Output: StreamOutput{Type: "proto:pcs.types.v1.Pairs"},
			},
		},
		{
			name: "basic store",
			rawYamlInput: `---
name: prices
kind: store
code:
  file: ./pricesState.wasm
inputs:
  - source: proto:sf.ethereum.type.v1.Block
  - store: pairs
output:
  updatePolicy: sum
  valueType: bigint`,
			expectedOutput: Module{
				Name:   "prices",
				Kind:   "store",
				Code:   Code{File: "./pricesState.wasm"},
				Inputs: []*Input{{Source: "proto:sf.ethereum.type.v1.Block"}, {Store: "pairs"}},
				Output: StreamOutput{UpdatePolicy: "sum", ValueType: "bigint"},
			},
		},
	}

	for _, tt := range tests {
		var tstream Module
		err := yaml.NewDecoder(strings.NewReader(tt.rawYamlInput)).Decode(&tstream)
		assert.NoError(t, err)
		assert.Equal(t, tt.expectedOutput, tstream)
	}
}

func TestStream_Signature_Basic(t *testing.T) {
	manifest, err := newWithoutLoad("./test/test_manifest.yaml")
	require.NoError(t, err)

	pairExtractorStream := manifest.Graph.modules[0]
	sig := pairExtractorStream.Signature(manifest.Graph)
	assert.Equal(t, "SAx2VACDM0U0cATBhdVLBEBWkhM=", base64.StdEncoding.EncodeToString(sig))
}

func TestStream_Signature_Composed(t *testing.T) {
	manifest, err := newWithoutLoad("./test/test_manifest.yaml")
	require.NoError(t, err)

	pairsStream := manifest.Graph.modules[1]
	sig := pairsStream.Signature(manifest.Graph)
	assert.Equal(t, "mJWxgtjCeH4ulmYN4fq3wVTUz8U=", base64.StdEncoding.EncodeToString(sig))
}

func TestStreamLinks_Streams(t *testing.T) {
	manifest, err := newWithoutLoad("./test/test_manifest.yaml")
	require.NoError(t, err)

	res, err := manifest.Graph.ModulesDownTo("reserves_extractor")
	require.NoError(t, err)
	fmt.Println(res)
}
