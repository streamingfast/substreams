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
	assert.GreaterOrEqual(t, len(manifest.Streams), 1)
	assert.Equal(t, manifest.GenesisBlock, 6809737)
}

func TestStreamYamlDecode(t *testing.T) {
	type test struct {
		name           string
		rawYamlInput   string
		expectedOutput Stream
	}

	tests := []test{
		{
			name: "basic mapper",
			rawYamlInput: `---
name: pairExtractor
kind: Mapper
code:
  file: ./pairExtractor.wasm
inputs:
  - proto:sf.ethereum.types.v1.Block
output:
  type: proto:pcs.types.v1.Pairs`,
			expectedOutput: Stream{
				Name:   "pairExtractor",
				Kind:   "Mapper",
				Code:   Code{File: "./pairExtractor.wasm"},
				Inputs: []string{"proto:sf.ethereum.types.v1.Block"},
				Output: StreamOutput{Type: "proto:pcs.types.v1.Pairs"},
			},
		},
		{
			name: "basic store",
			rawYamlInput: `---
name: prices
kind: StateBuilder
code:
  file: ./pricesState.wasm
inputs:
  - proto:sf.ethereum.types.v1.Block
  - store:pairs
output:
  storeMergeStrategy: LAST_KEY`,
			expectedOutput: Stream{
				Name:   "prices",
				Kind:   "StateBuilder",
				Code:   Code{File: "./pricesState.wasm"},
				Inputs: []string{"proto:sf.ethereum.types.v1.Block", "store:pairs"},
				Output: StreamOutput{StoreMergeStrategy: "LAST_KEY"},
			},
		},
	}

	for _, tt := range tests {
		var tstream Stream
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
	assert.Equal(t, "6udco/e0Cs9WlTak8aVqDEaLf5w=", base64.StdEncoding.EncodeToString(sig))
}

func TestStream_Signature_Composed(t *testing.T) {
	manifest, err := newWithoutLoad("./test/test_manifest.yaml")
	require.NoError(t, err)

	pairsStream := manifest.Graph.streams["pairs"]
	sig := pairsStream.Signature(manifest.Graph)
	assert.Equal(t, "pHzwn8h6952T5++c0mTLarIwb54=", base64.StdEncoding.EncodeToString(sig))
}

func TestStreamLinks_Streams(t *testing.T) {
	manifest, err := newWithoutLoad("./test/test_manifest.yaml")
	require.NoError(t, err)

	manifest.Graph.StreamsFor("prices")
}

func TestStreamLinks_StreamsFor(t *testing.T) {
	streamGraph := &StreamsGraph{
		streams: map[string]*Stream{
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
		links: map[string][]*Stream{
			"A": {&Stream{Name: "B"}, &Stream{Name: "C"}},
			"B": {&Stream{Name: "D"}, &Stream{Name: "E"}, &Stream{Name: "F"}},
			"C": {&Stream{Name: "F"}},
			"D": {},
			"E": {},
			"F": {&Stream{Name: "G"}, &Stream{Name: "H"}},
			"G": {},
			"H": {},
			"I": {&Stream{Name: "H"}},
		},
	}

	res, err := streamGraph.StreamsFor("A")
	assert.NoError(t, err)

	_ = res
}

func TestStreamLinks_GroupedStreamsFor(t *testing.T) {
	streamGraph := &StreamsGraph{
		streams: map[string]*Stream{
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
		links: map[string][]*Stream{
			"A": {&Stream{Name: "B"}, &Stream{Name: "C"}},
			"B": {&Stream{Name: "D"}, &Stream{Name: "E"}, &Stream{Name: "F"}},
			"C": {&Stream{Name: "F"}},
			"D": {},
			"E": {},
			"F": {&Stream{Name: "G"}, &Stream{Name: "H"}},
			"G": {},
			"H": {},
			"I": {&Stream{Name: "H"}},
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
