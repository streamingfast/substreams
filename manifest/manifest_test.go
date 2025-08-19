package manifest

import (
	"os"
	"strings"
	"testing"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestManifest_YamlUnmarshal(t *testing.T) {
	manifest, err := decodeYamlManifestFromFile("./test/test_manifest.yaml", ".")
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
		{
			name: "basic module with use",
			rawYamlInput: `---
name: use_module
use: converter:dbout_to_graphout
inputs:
  - source: proto:sf.ethereum.type.v1.Block
  - store: pairs
  - map: map_clocks
`,
			expectedOutput: Module{
				Name:   "use_module",
				Use:    "converter:dbout_to_graphout",
				Inputs: []*Input{{Source: "proto:sf.ethereum.type.v1.Block"}, {Store: "pairs"}, {Map: "map_clocks"}},
			},
		},
		{
			name: "basic index",
			rawYamlInput: `---
name: basic_index
kind: blockIndex
output:
    type: proto:sf.substreams.index.v1.Keys
`,
			expectedOutput: Module{
				Kind:   ModuleKindBlockIndex,
				Name:   "basic_index",
				Output: StreamOutput{Type: "proto:sf.substreams.index.v1.Keys"},
			},
		},
		{
			name: "basic with block filter string",
			rawYamlInput: `---
name: bf_module
kind: map
blockFilter:
 module: basic_index
 query:
   string: this is my query
output:
    type: proto:sf.substreams.database.changes.v1
`,

			expectedOutput: Module{
				Kind:   ModuleKindMap,
				Name:   "bf_module",
				Output: StreamOutput{Type: "proto:sf.substreams.database.changes.v1"},
				BlockFilter: &BlockFilter{
					Module: "basic_index",
					Query:  BlockFilterQuery{String: "this is my query"},
				},
			},
		},
		{
			name: "basic with block filter from params",
			rawYamlInput: `---
name: bf_module
kind: map
blockFilter:
 module: basic_index
 query:
   params: true
inputs:
  - params: string
output:
    type: proto:sf.substreams.database.changes.v1
`,

			expectedOutput: Module{
				Kind:   ModuleKindMap,
				Name:   "bf_module",
				Inputs: []*Input{{Params: "string"}},
				Output: StreamOutput{Type: "proto:sf.substreams.database.changes.v1"},
				BlockFilter: &BlockFilter{
					Module: "basic_index",
					Query:  BlockFilterQuery{Params: true},
				},
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
	reader := MustNewReader("./test/test_manifest.yaml")
	pkgBundle, err := reader.Read()
	require.NoError(t, err)
	require.NotNil(t, pkgBundle)

	pkg := pkgBundle.Package
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

func TestParseFoundationalStoreIdentifier(t *testing.T) {
	type test struct {
		name           string
		input          string
		expectedOutput struct {
			packageName string
			version     string
			isShortcut  bool
		}
		expectError string
	}

	tests := []test{
		{
			name:  "valid package notation",
			input: "account-owners@v1.0.0",
			expectedOutput: struct {
				packageName string
				version     string
				isShortcut  bool
			}{
				packageName: "account-owners",
				version:     "v1.0.0",
				isShortcut:  true,
			},
		},
		{
			name:  "valid single word package",
			input: "tokens@v2.1.3",
			expectedOutput: struct {
				packageName string
				version     string
				isShortcut  bool
			}{
				packageName: "tokens",
				version:     "v2.1.3",
				isShortcut:  true,
			},
		},
		{
			name:  "grpc endpoint without @",
			input: "grpc://localhost:50051",
			expectedOutput: struct {
				packageName string
				version     string
				isShortcut  bool
			}{
				packageName: "grpc://localhost:50051",
				version:     "",
				isShortcut:  false,
			},
		},
		{
			name:        "empty package name",
			input:       "@v1.0.0",
			expectError: "package name cannot be empty",
		},
		{
			name:        "empty version",
			input:       "account-owners@",
			expectError: "version cannot be empty",
		},
		{
			name:        "invalid package name with uppercase",
			input:       "Account-Owners@v1.0.0",
			expectError: "package name \"Account-Owners\" is invalid",
		},
		{
			name:        "invalid package name with underscore",
			input:       "account_owners@v1.0.0",
			expectError: "package name \"account_owners\" is invalid",
		},
		{
			name:        "version without v prefix",
			input:       "account-owners@1.0.0",
			expectError: "version \"1.0.0\" must start with 'v'",
		},
		{
			name:        "invalid version vlatest",
			input:       "account-owners@vlatest",
			expectError: "version \"vlatest\" is not supported",
		},
		{
			name:        "multiple validation errors",
			input:       "Invalid_Package@1.0.0",
			expectError: "package name \"Invalid_Package\" is invalid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			packageName, version, isShortcut, err := parseFoundationalStoreIdentifier(tt.input)

			if tt.expectError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectError)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectedOutput.packageName, packageName)
				assert.Equal(t, tt.expectedOutput.version, version)
				assert.Equal(t, tt.expectedOutput.isShortcut, isShortcut)
			}
		})
	}
}

func TestResolveFoundationalStoreEndpoint(t *testing.T) {
	// Save original environment
	originalEndpoint := os.Getenv("FOUNDATIONAL_STORE_ENDPOINT")
	originalEnvironment := os.Getenv("DEPLOYMENT_ENVIRONMENT")

	// Cleanup function
	cleanup := func() {
		if originalEndpoint != "" {
			os.Setenv("FOUNDATIONAL_STORE_ENDPOINT", originalEndpoint)
		} else {
			os.Unsetenv("FOUNDATIONAL_STORE_ENDPOINT")
		}
		if originalEnvironment != "" {
			os.Setenv("DEPLOYMENT_ENVIRONMENT", originalEnvironment)
		} else {
			os.Unsetenv("DEPLOYMENT_ENVIRONMENT")
		}
	}
	defer cleanup()

	type test struct {
		name           string
		envEndpoint    string
		envEnvironment string
		expectedOutput string
	}

	tests := []test{
		{
			name:           "custom endpoint via env var",
			envEndpoint:    "grpc://custom-host:9999",
			envEnvironment: "production",
			expectedOutput: "grpc://custom-host:9999",
		},
		{
			name:           "local environment",
			envEndpoint:    "",
			envEnvironment: "local",
			expectedOutput: "grpc://localhost:50051",
		},
		{
			name:           "dev environment",
			envEndpoint:    "",
			envEnvironment: "dev",
			expectedOutput: "grpc://localhost:50051",
		},
		{
			name:           "development environment",
			envEndpoint:    "",
			envEnvironment: "development",
			expectedOutput: "grpc://localhost:50051",
		},
		{
			name:           "staging environment",
			envEndpoint:    "",
			envEnvironment: "staging",
			expectedOutput: "grpc://foundational-store-staging:10016",
		},
		{
			name:           "stage environment",
			envEndpoint:    "",
			envEnvironment: "stage",
			expectedOutput: "grpc://foundational-store-staging:10016",
		},
		{
			name:           "production environment",
			envEndpoint:    "",
			envEnvironment: "production",
			expectedOutput: "grpc://foundational-store:10016",
		},
		{
			name:           "prod environment",
			envEndpoint:    "",
			envEnvironment: "prod",
			expectedOutput: "grpc://foundational-store:10016",
		},
		{
			name:           "unknown environment defaults to production",
			envEndpoint:    "",
			envEnvironment: "unknown",
			expectedOutput: "grpc://foundational-store:10016",
		},
		{
			name:           "empty environment defaults to production",
			envEndpoint:    "",
			envEnvironment: "",
			expectedOutput: "grpc://foundational-store:10016",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set environment variables
			if tt.envEndpoint != "" {
				os.Setenv("FOUNDATIONAL_STORE_ENDPOINT", tt.envEndpoint)
			} else {
				os.Unsetenv("FOUNDATIONAL_STORE_ENDPOINT")
			}
			os.Setenv("DEPLOYMENT_ENVIRONMENT", tt.envEnvironment)

			result := resolveFoundationalStoreEndpoint("account-owners", "v1.0.0")
			assert.Equal(t, tt.expectedOutput, result)
		})
	}
}

func TestInputResolveFoundationalStoreEndpoint(t *testing.T) {
	tests := []struct {
		name           string
		inputStore     string
		expectEndpoint string
	}{
		{
			name:           "grpc endpoint passthrough",
			inputStore:     "grpc://localhost:50051",
			expectEndpoint: "grpc://localhost:50051",
		},
		{
			name:           "grpc endpoint with custom port",
			inputStore:     "grpc://foundational-store:9999",
			expectEndpoint: "grpc://foundational-store:9999",
		},
		{
			name:           "package notation resolves to default",
			inputStore:     "account-owners@v1.0.0",
			expectEndpoint: "grpc://foundational-store:10016", // default when no env vars set
		},
		{
			name:           "invalid format returns as-is",
			inputStore:     "invalid-format-no-grpc-or-at",
			expectEndpoint: "invalid-format-no-grpc-or-at",
		},
	}

	// Clear environment variables for consistent test results
	os.Unsetenv("FOUNDATIONAL_STORE_ENDPOINT")
	os.Unsetenv("DEPLOYMENT_ENVIRONMENT")

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := &Input{
				FoundationalStore: tt.inputStore,
			}
			result := input.resolveFoundationalStoreEndpoint()
			assert.Equal(t, tt.expectEndpoint, result)
		})
	}
}

func TestInputParseFoundationalStore(t *testing.T) {
	tests := []struct {
		name        string
		input       Input
		expectError string
	}{
		{
			name: "valid grpc endpoint",
			input: Input{
				FoundationalStore: "grpc://localhost:50051",
			},
			expectError: "",
		},
		{
			name: "valid package notation",
			input: Input{
				FoundationalStore: "account-owners@v1.0.0",
			},
			expectError: "",
		},
		{
			name: "invalid package notation - empty package",
			input: Input{
				FoundationalStore: "@v1.0.0",
			},
			expectError: "package name cannot be empty",
		},
		{
			name: "invalid package notation - empty version",
			input: Input{
				FoundationalStore: "account-owners@",
			},
			expectError: "version cannot be empty",
		},
		{
			name: "invalid package notation - bad package name",
			input: Input{
				FoundationalStore: "Account_Owners@v1.0.0",
			},
			expectError: "package name \"Account_Owners\" is invalid",
		},
		{
			name: "invalid package notation - bad version",
			input: Input{
				FoundationalStore: "account-owners@1.0.0",
			},
			expectError: "version \"1.0.0\" must start with 'v'",
		},
		{
			name: "unsupported format",
			input: Input{
				FoundationalStore: "http://not-supported",
			},
			expectError: "unsupported format. Use either package notation (package@version) or gRPC endpoint (grpc://host:port)",
		},
		{
			name: "completely invalid format",
			input: Input{
				FoundationalStore: "totally-random-string",
			},
			expectError: "unsupported format. Use either package notation (package@version) or gRPC endpoint (grpc://host:port)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.input.parseFoundationalStore()

			if tt.expectError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectError)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestFoundationalStorePackageNameRegexp(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		isValid bool
	}{
		{
			name:    "valid single word",
			input:   "accounts",
			isValid: true,
		},
		{
			name:    "valid hyphenated",
			input:   "account-owners",
			isValid: true,
		},
		{
			name:    "valid multiple hyphens",
			input:   "token-account-owners",
			isValid: true,
		},
		{
			name:    "invalid with uppercase",
			input:   "Account-owners",
			isValid: false,
		},
		{
			name:    "invalid with underscore",
			input:   "account_owners",
			isValid: false,
		},
		{
			name:    "invalid with numbers",
			input:   "account2owners",
			isValid: false,
		},
		{
			name:    "invalid starting with hyphen",
			input:   "-account-owners",
			isValid: false,
		},
		{
			name:    "invalid ending with hyphen",
			input:   "account-owners-",
			isValid: false,
		},
		{
			name:    "invalid empty",
			input:   "",
			isValid: false,
		},
		{
			name:    "invalid consecutive hyphens",
			input:   "account--owners",
			isValid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := foundationalStorePackageNameRegexp.MatchString(tt.input)
			assert.Equal(t, tt.isValid, result, "Expected %s to be valid=%v", tt.input, tt.isValid)
		})
	}
}
