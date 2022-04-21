package manifest

import (
	"strings"
	"testing"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestManifest_YamlUnmarshal(t *testing.T) {
	_, manifest, err := DecodeYamlManifestFromFile("./test/test_manifest.yaml")
	assert.NoError(t, err)
	assert.GreaterOrEqual(t, len(manifest.Modules), 1)
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
updatePolicy: sum
valueType: bigint
code:
  file: ./pricesState.wasm
inputs:
  - source: proto:sf.ethereum.type.v1.Block
  - store: pairs
`,
			expectedOutput: Module{
				Name:         "prices",
				Kind:         "store",
				UpdatePolicy: "sum",
				ValueType:    "bigint",

				Code:   Code{File: "./pricesState.wasm"},
				Inputs: []*Input{{Source: "proto:sf.ethereum.type.v1.Block"}, {Store: "pairs"}},
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

//func TestStream_Signature_Basic(t *testing.T) {
//	manifest, err := newWithoutLoad("./test/test_manifest.yaml")
//	require.NoError(t, err)
//
//	pairExtractorStream := manifest.Graph.modules[0]
//	sig := pairExtractorStream.MonduleSignature(manifest.Graph)
//	assert.Equal(t, "SAx2VACDM0U0cATBhdVLBEBWkhM=", base64.StdEncoding.EncodeToString(sig))
//}
//
//func TestStream_Signature_Composed(t *testing.T) {
//	manifest, err := newWithoutLoad("./test/test_manifest.yaml")
//	require.NoError(t, err)
//
//	pairsStream := manifest.Graph.modules[1]
//	sig := pairsStream.MonduleSignature(manifest.Graph)
//	assert.Equal(t, "mJWxgtjCeH4ulmYN4fq3wVTUz8U=", base64.StdEncoding.EncodeToString(sig))
//}

func TestManifest_ToProto(t *testing.T) {
	manifest, err := newWithoutLoad("./test/test_manifest.yaml")
	require.NoError(t, err)

	pbManifest, err := manifest.ToProto()
	require.NoError(t, err)

	require.Equal(t, 1, len(pbManifest.ModulesCode))

	require.Equal(t, 4, len(pbManifest.Modules))

	module := pbManifest.Modules[0]
	require.Equal(t, "pair_extractor", module.Name)
	require.Equal(t, "map_pairs", module.GetWasmCode().Entrypoint)
	require.Equal(t, uint32(0), module.GetWasmCode().Index)
	require.Equal(t, "proto:pcs.types.v1.Pairs", module.Output.Type)

	module = pbManifest.Modules[1]
	require.Equal(t, "pairs", module.Name)
	require.Equal(t, "build_pairs_state", module.GetWasmCode().Entrypoint)
	require.Equal(t, uint32(0), module.GetWasmCode().Index)
	require.Equal(t, 1, len(module.Inputs))
	require.Equal(t, "pair_extractor", module.Inputs[0].GetMap().ModuleName)
	require.Equal(t, pbsubstreams.Module_KindStore_UPDATE_POLICY_REPLACE, module.GetKindStore().UpdatePolicy)
	require.Nil(t, module.Output)

	module = pbManifest.Modules[2]
	require.Equal(t, "reserves_extractor", module.Name)
	require.Equal(t, "map_reserves", module.GetWasmCode().Entrypoint)
	require.Equal(t, uint32(0), module.GetWasmCode().Index)
	require.Equal(t, 2, len(module.Inputs))
	require.Equal(t, "sf.ethereum.type.v1.Block", module.Inputs[0].GetSource().Type)
	require.Equal(t, "pairs", module.Inputs[1].GetStore().ModuleName)
	require.Equal(t, "proto:pcs.types.v1.Reserves", module.Output.Type)

	module = pbManifest.Modules[3]
	require.Equal(t, "block_to_tokens", module.Name)
	require.Equal(t, "map_block_to_tokens", module.GetWasmCode().Entrypoint)
	require.Equal(t, uint32(0), module.GetWasmCode().Index)
	require.Equal(t, "proto:sf.substreams.tokens.v1.Tokens", module.Output.Type)
}
