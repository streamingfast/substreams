package manifest

import (
	"encoding/base64"
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
	assert.Equal(t, manifest.GenesisBlock, 6809737)
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
  file: ./pairExtractor.wasm
inputs:
  - source: proto:sf.ethereum.codec.v1.Block
output:
  type: proto:pcs.types.v1.Pairs`,
			expectedOutput: Module{
				Name:   "pairExtractor",
				Kind:   "map",
				Code:   Code{File: "./pairExtractor.wasm"},
				Inputs: []*Input{{Source: "proto:sf.ethereum.codec.v1.Block"}},
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
  - source: proto:sf.ethereum.codec.v1.Block
  - store: pairs
output:
  updatePolicy: sum
  valueType: bigint`,
			expectedOutput: Module{
				Name:   "prices",
				Kind:   "store",
				Code:   Code{File: "./pricesState.wasm"},
				Inputs: []*Input{{Source: "proto:sf.ethereum.codec.v1.Block"}, {Store: "pairs"}},
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

	pairExtractorStream := manifest.Graph.streams["pairExtractor"]
	sig := pairExtractorStream.Signature(manifest.Graph)
	assert.Equal(t, "9Sdn9wyVddnTCFAMRwhgCXrJ3+k=", base64.StdEncoding.EncodeToString(sig))
}

func TestStream_Signature_Composed(t *testing.T) {
	manifest, err := newWithoutLoad("./test/test_manifest.yaml")
	require.NoError(t, err)

	pairsStream := manifest.Graph.streams["pairs"]
	sig := pairsStream.Signature(manifest.Graph)
	assert.Equal(t, "LKtX3dNYKlsZTmhNd/3qMCYx7E4=", base64.StdEncoding.EncodeToString(sig))
}

func TestStreamLinks_Streams(t *testing.T) {
	manifest, err := newWithoutLoad("./test/test_manifest.yaml")
	require.NoError(t, err)

	manifest.Graph.StreamsFor("prices")
}

func TestStreamLinks_StreamsFor(t *testing.T) {
	streamGraph := &StreamsGraph{
		streams: map[string]*Module{
			"A": {Name: "A"},
			"B": {Name: "B"},
			"C": {Name: "C"},
			"D": {Name: "D"},
			"E": {Name: "E"},
			"F": {Name: "F"},
			"G": {Name: "G"},
			"H": {Name: "H"},
			"I": {Name: "I"},
		},
		links: map[string][]*Module{
			"A": {&Module{Name: "B"}, &Module{Name: "C"}},
			"B": {&Module{Name: "D"}, &Module{Name: "E"}, &Module{Name: "F"}},
			"C": {&Module{Name: "F"}},
			"D": {},
			"E": {},
			"F": {&Module{Name: "G"}, &Module{Name: "H"}},
			"G": {},
			"H": {},
			"I": {&Module{Name: "H"}},
		},
	}

	res, err := streamGraph.StreamsFor("A")
	assert.NoError(t, err)

	_ = res
}

func TestStreamLinks_GroupedStreamsFor(t *testing.T) {
	streamGraph := &StreamsGraph{
		streams: map[string]*Module{
			"A": {Name: "A"},
			"B": {Name: "B"},
			"C": {Name: "C"},
			"D": {Name: "D"},
			"E": {Name: "E"},
			"F": {Name: "F"},
			"G": {Name: "G"},
			"H": {Name: "H"},
			"I": {Name: "I"},
		},
		links: map[string][]*Module{
			"A": {&Module{Name: "B"}, &Module{Name: "C"}},
			"B": {&Module{Name: "D"}, &Module{Name: "E"}, &Module{Name: "F"}},
			"C": {&Module{Name: "F"}},
			"D": {},
			"E": {},
			"F": {&Module{Name: "G"}, &Module{Name: "H"}},
			"G": {},
			"H": {},
			"I": {&Module{Name: "H"}},
		},
	}

	res, err := streamGraph.GroupedStreamsFor("A")
	assert.NoError(t, err)

	groups := make([][]string, len(res), len(res))
	for i, r := range res {
		sr := make([]string, 0, len(r))
		for _, i := range r {
			sr = append(sr, i.String())
		}
		groups[i] = sr
	}

	assertStringSliceContainsValues(t, groups[0], []string{"G", "H"})
	assertStringSliceContainsValues(t, groups[1], []string{"D", "E", "F"})
	assertStringSliceContainsValues(t, groups[2], []string{"B", "C"})
	assertStringSliceContainsValues(t, groups[3], []string{"A"})
}

func assertStringSliceContainsValues(t *testing.T, slice []string, values []string) {
	sliceMap := map[string]struct{}{}
	for _, v := range slice {
		sliceMap[v] = struct{}{}
	}

	valuesMap := map[string]struct{}{}
	for _, v := range values {
		valuesMap[v] = struct{}{}
	}

	for kv := range valuesMap {
		if _, ok := sliceMap[kv]; !ok {
			t.Errorf("value missing")
		}
	}

	for ks := range sliceMap {
		if _, ok := valuesMap[ks]; !ok {
			t.Errorf("value missing")
		}
	}
}
