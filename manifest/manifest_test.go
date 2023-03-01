package manifest

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

func TestManifest_YamlUnmarshal(t *testing.T) {
	manifest, err := decodeYamlManifestFromFile("./test/test_manifest.yaml")
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
binary: bob
inputs:
  - source: proto:sf.ethereum.type.v1.Block
output:
  type: proto:pcs.types.v1.Pairs`,
			expectedOutput: Module{
				Name:   "pairExtractor",
				Kind:   "map",
				Binary: "bob",
				Inputs: []*Input{{Source: "proto:sf.ethereum.type.v1.Block"}},
				Output: StreamOutput{Type: "proto:pcs.types.v1.Pairs"},
			},
		},
		{
			name: "basic store",
			rawYamlInput: `---
name: prices
kind: store
updatePolicy: add
valueType: bigint
inputs:
  - source: proto:sf.ethereum.type.v1.Block
  - store: pairs
`,
			expectedOutput: Module{
				Name:         "prices",
				Kind:         "store",
				UpdatePolicy: "add",
				ValueType:    "bigint",
				Inputs:       []*Input{{Source: "proto:sf.ethereum.type.v1.Block"}, {Store: "pairs"}},
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
	reader := NewReader("./test/test_manifest.yaml")
	pkg, err := reader.Read()
	require.NoError(t, err)

	pbManifest := pkg.Modules

	require.Equal(t, 1, len(pbManifest.Binaries))

	require.Equal(t, 4, len(pbManifest.Modules))

	module := pbManifest.Modules[0]
	require.Equal(t, "map_pairs", module.Name)
	require.Equal(t, "map_pairs", module.BinaryEntrypoint)
	require.Equal(t, uint32(0), module.BinaryIndex)
	require.Equal(t, 2, len(module.Inputs))
	require.Equal(t, "my default params", module.Inputs[0].GetParams().Value)
	require.Equal(t, "sf.ethereum.type.v1.Block", module.Inputs[1].GetSource().Type)
	require.Equal(t, "proto:pcs.types.v1.Pairs", module.Output.Type)

	module = pbManifest.Modules[1]
	require.Equal(t, "build_pairs_state", module.Name)
	require.Equal(t, "build_pairs_state", module.BinaryEntrypoint)
	require.Equal(t, uint32(0), module.BinaryIndex)
	require.Equal(t, 1, len(module.Inputs))
	require.Equal(t, "map_pairs", module.Inputs[0].GetMap().ModuleName)
	require.Equal(t, pbsubstreams.Module_KindStore_UPDATE_POLICY_SET, module.GetKindStore().UpdatePolicy)
	require.Nil(t, module.Output)

	module = pbManifest.Modules[2]
	require.Equal(t, "map_reserves", module.Name)
	require.Equal(t, "map_reserves", module.BinaryEntrypoint)
	require.Equal(t, uint32(0), module.BinaryIndex)
	require.Equal(t, 2, len(module.Inputs))
	require.Equal(t, "sf.ethereum.type.v1.Block", module.Inputs[0].GetSource().Type)
	require.Equal(t, "build_pairs_state", module.Inputs[1].GetStore().ModuleName)
	require.Equal(t, "proto:pcs.types.v1.Reserves", module.Output.Type)

	module = pbManifest.Modules[3]
	require.Equal(t, "map_block_to_tokens", module.Name)
	require.Equal(t, "map_block_to_tokens", module.BinaryEntrypoint)
	require.Equal(t, uint32(0), module.BinaryIndex)
	require.Equal(t, "proto:sf.substreams.tokens.v1.Tokens", module.Output.Type)

	require.Equal(t, "antelope", pkg.Network)
	require.Equal(t, "pcs.services.v1.WASMQueryService", pkg.SinkConfig.TypeUrl)
	require.Equal(t, "map_block_to_tokens", pkg.SinkModule)
	//require.Equal(t, "begin of json config for sink", reader.sinkConfigJSON)
	require.Len(t, pkg.SinkConfig.Value, 2178)
	addSomePancakes := reader.sinkConfigDynamicMessage.GetFieldByName("add_some_pancakes").(bool)
	require.True(t, addSomePancakes)
	someBytes := reader.sinkConfigDynamicMessage.GetFieldByName("some_bytes").([]byte)
	require.Equal(t, "specVersion:", string(someBytes)[:12])
	someString := reader.sinkConfigDynamicMessage.GetFieldByName("some_string").(string)
	require.Equal(t, "specVersion:", someString[:12])

}

//
//type testSinkConfig struct {
//	state         protoimpl.MessageState
//	sizeCache     protoimpl.SizeCache
//	unknownFields protoimpl.UnknownFields
//
//	AddSomePancakes bool `protobuf:"varint,1,opt,name=add_some_pancakes,json=addSomePancakes,proto3" json:"add_some_pancakes,omitempty"`
//}
//
//func (x *testSinkConfig) Reset()                             { *x = testSinkConfig{} }
//func (x *testSinkConfig) String() string                     { return "testSinkConfig" }
//func (*testSinkConfig) ProtoMessage()                        {}
//func (x *testSinkConfig) ProtoReflect() protoreflect.Message { panic("unimplemented") }
