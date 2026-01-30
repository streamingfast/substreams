package manifest

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
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

func TestParseFoundationalStoreIdentifier(t *testing.T) {
	type out struct {
		packageName string
		version     string
		isShortcut  bool
	}
	type test struct {
		name        string
		input       string
		expected    out
		expectError string
	}

	tests := []test{
		{
			name:  "valid package notation",
			input: "account-owners@v1.0.0",
			expected: out{
				packageName: "account-owners",
				version:     "v1.0.0",
				isShortcut:  true,
			},
		},
		{
			name:  "valid single word package",
			input: "tokens@v2.1.3",
			expected: out{
				packageName: "tokens",
				version:     "v2.1.3",
				isShortcut:  true,
			},
		},
		{
			name:        "empty package name",
			input:       "@v1.0.0",
			expectError: "does not match regexp",
		},
		{
			name:        "empty version becomes latest (then rejected at parseFoundationalStore time)",
			input:       "account-owners@",
			expected:    out{packageName: "account-owners", version: "latest", isShortcut: true},
			expectError: "",
		},
		{
			name:        "invalid version (not semver)",
			input:       "account-owners@1.0.0",
			expectError: "not valid Semver",
		},
		{
			name:        "multiple validation issues",
			input:       "Invalid Package@1.0.0",
			expectError: "does not match regexp",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pkg, ver, shortcut, validationErr := ParseShortPackageIdentifier(tt.input)

			if tt.expectError != "" {
				require.Error(t, validationErr)
				assert.Contains(t, validationErr.Error(), tt.expectError)
				return
			}

			require.NoError(t, validationErr)
			assert.Equal(t, tt.expected.packageName, pkg)
			assert.Equal(t, tt.expected.version, ver)
			assert.Equal(t, tt.expected.isShortcut, shortcut)
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
			name: "grpc_no_longer_supported_here",
			input: Input{
				FoundationalStore: "grpc://localhost:50051",
			},
			expectError: "invalid foundational-store identifier",
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
			expectError: "invalid foundational-store identifier",
		},
		{
			name: "invalid package notation - empty version (interpreted as latest, which is forbidden)",
			input: Input{
				FoundationalStore: "account-owners@",
			},
			expectError: `version "latest" is not supported`,
		},
		{
			name: "invalid package notation - bad version",
			input: Input{
				FoundationalStore: "account-owners@1.0.0",
			},
			expectError: "not valid Semver",
		},
		{
			name: "unsupported_format_http",
			input: Input{
				FoundationalStore: "http://not-supported",
			},
			expectError: "invalid foundational-store identifier",
		},
		{
			name: "completely invalid format",
			input: Input{
				FoundationalStore: "totally-random-string",
			},
			expectError: "expected package@version",
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

func TestFoundationalStore_HashBehavior(t *testing.T) {
	mkModule := func(identifier string) *pbsubstreams.Module {
		return &pbsubstreams.Module{
			Name: "m",
			Kind: &pbsubstreams.Module_KindMap_{
				KindMap: &pbsubstreams.Module_KindMap{
					OutputType: "proto:sf.substreams.test.v1.Dummy",
				},
			},
			Inputs: []*pbsubstreams.Module_Input{
				{
					Input: &pbsubstreams.Module_Input_FoundationalStore{
						FoundationalStore: &pbsubstreams.Module_FoundationalStore{
							Identifier: identifier,
						},
					},
				},
			},
		}
	}
	mkPackage := func(identifier string) *pbsubstreams.Package {
		return &pbsubstreams.Package{
			Version: 1,
			Modules: &pbsubstreams.Modules{
				Modules: []*pbsubstreams.Module{mkModule(identifier)},
			},
			Network: "testnet",
		}
	}
	hash := func(m proto.Message) string {
		b, err := proto.MarshalOptions{Deterministic: true}.Marshal(m)
		require.NoError(t, err)
		sum := sha256.Sum256(b)
		return hex.EncodeToString(sum[:])
	}

	type tc struct {
		name        string
		scope       string
		idA         string
		idB         string
		expectEqual bool
	}
	tests := []tc{
		// different hash
		{
			name:        "module: version change",
			scope:       "module",
			idA:         "account-owners@v1.0.0",
			idB:         "account-owners@v1.0.1",
			expectEqual: false,
		},
		// different hash
		{
			name:        "package: version change",
			scope:       "package",
			idA:         "account-owners@v1.0.0",
			idB:         "account-owners@v1.0.1",
			expectEqual: false,
		},
		// same hash
		{
			name:        "module: same identifier",
			scope:       "module",
			idA:         "account-owners@v1.2.3",
			idB:         "account-owners@v1.2.3",
			expectEqual: true,
		},
		// same hash
		{
			name:        "package: same identifier",
			scope:       "package",
			idA:         "account-owners@v1.2.3",
			idB:         "account-owners@v1.2.3",
			expectEqual: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var hA, hB string
			switch tt.scope {
			case "module":
				hA = hash(mkModule(tt.idA))
				hB = hash(mkModule(tt.idB))
			case "package":
				hA = hash(mkPackage(tt.idA))
				hB = hash(mkPackage(tt.idB))
			default:
				t.Fatalf("unknown scope %q", tt.scope)
			}

			if tt.expectEqual {
				require.Equal(t, hA, hB)
			} else {
				require.NotEqual(t, hA, hB)
			}
		})
	}
}

func TestDescriptorSetEntry_UnmarshalYAML(t *testing.T) {
	tests := []struct {
		name        string
		yaml        string
		expected    DescriptorSetEntry
		expectError bool
		errorMsg    string
	}{
		{
			name: "old format with full module path and semver version",
			yaml: `module: buf.build/streamingfast/substreams-sink-sql
version: v0.1.0`,
			expected: DescriptorSetEntry{
				Module:  "buf.build/streamingfast/substreams-sink-sql",
				Version: "v0.1.0",
			},
			expectError: false,
		},
		{
			name: "full path with @version notation (semver inline)",
			yaml: `module: buf.build/streamingfast/substreams-sink-sql@v0.1.0`,
			expected: DescriptorSetEntry{
				Module:  "buf.build/streamingfast/substreams-sink-sql",
				Version: "v0.1.0",
			},
			expectError: false,
		},
		{
			name: "full path with @version notation (different package)",
			yaml: `module: buf.build/streamingfast/substreams-foundational-store@v1.2.3`,
			expected: DescriptorSetEntry{
				Module:  "buf.build/streamingfast/substreams-foundational-store",
				Version: "v1.2.3",
			},
			expectError: false,
		},
		{
			name: "without version (resolves to latest implicitly)",
			yaml: `module: buf.build/streamingfast/substreams-sink-sql`,
			expected: DescriptorSetEntry{
				Module:  "buf.build/streamingfast/substreams-sink-sql",
				Version: "",
			},
			expectError: false,
		},
		{
			name: "separate version with latest (allowed)",
			yaml: `module: buf.build/streamingfast/substreams-sink-sql
version: latest`,
			expected: DescriptorSetEntry{
				Module:  "buf.build/streamingfast/substreams-sink-sql",
				Version: "",
			},
			expectError: false,
		},
		{
			name: "error: version specified both inline and separate",
			yaml: `module: buf.build/streamingfast/substreams-sink-sql@v0.1.0
version: v0.2.0`,
			expectError: true,
			errorMsg:    "cannot specify version both inline (with '@') and in the separate 'version' field",
		},
		{
			name:        "error: @latest is NOT allowed inline",
			yaml:        `module: buf.build/streamingfast/substreams-sink-sql@latest`,
			expectError: true,
			errorMsg:    "'@latest' is not allowed in inline notation; use 'version: latest' or omit the version",
		},
		{
			name:        "error: empty version after @",
			yaml:        `module: buf.build/streamingfast/substreams-sink-sql@`,
			expectError: true,
			errorMsg:    "empty version after '@'",
		},
		{
			name:        "error: invalid semver version inline",
			yaml:        `module: buf.build/streamingfast/substreams-sink-sql@invalid-version`,
			expectError: true,
			errorMsg:    "is not valid semantic version",
		},
		{
			name:        "error: version with multiple @ characters",
			yaml:        `module: buf.build/streamingfast/substreams-sink-sql@v0.1.0@extra`,
			expectError: true,
			errorMsg:    "should not contain '@'",
		},
		{
			name: "with symbols in separate version format",
			yaml: `module: buf.build/streamingfast/substreams-sink-sql
version: v0.1.0
symbols:
  - sf.substreams.sink.sql.v1.Table`,
			expected: DescriptorSetEntry{
				Module:  "buf.build/streamingfast/substreams-sink-sql",
				Version: "v0.1.0",
				Symbols: []string{"sf.substreams.sink.sql.v1.Table"},
			},
			expectError: false,
		},
		{
			name: "with symbols and inline semver",
			yaml: `module: buf.build/streamingfast/substreams-sink-sql@v0.1.0
symbols:
  - sf.substreams.sink.sql.v1.Table`,
			expected: DescriptorSetEntry{
				Module:  "buf.build/streamingfast/substreams-sink-sql",
				Version: "v0.1.0",
				Symbols: []string{"sf.substreams.sink.sql.v1.Table"},
			},
			expectError: false,
		},
		{
			name: "error: separate non-semver version (not latest)",
			yaml: `module: buf.build/streamingfast/substreams-sink-sql
version: 1.2`,
			expectError: true,
			errorMsg:    "is not valid semantic version (or 'latest')",
		},
		{
			name: "buf commit reference inline (32 hex chars)",
			yaml: `module: buf.build/streamingfast/substreams-foundational-store@f3ab5976b9ba4f9bac28b26271fca7d7`,
			expected: DescriptorSetEntry{
				Module:  "buf.build/streamingfast/substreams-foundational-store",
				Version: "f3ab5976b9ba4f9bac28b26271fca7d7",
			},
			expectError: false,
		},
		{
			name: "buf commit reference in separate version field",
			yaml: `module: buf.build/streamingfast/substreams-foundational-store
version: f3ab5976b9ba4f9bac28b26271fca7d7`,
			expected: DescriptorSetEntry{
				Module:  "buf.build/streamingfast/substreams-foundational-store",
				Version: "f3ab5976b9ba4f9bac28b26271fca7d7",
			},
			expectError: false,
		},
		{
			name: "buf commit reference with symbols",
			yaml: `module: buf.build/streamingfast/substreams-foundational-store@f3ab5976b9ba4f9bac28b26271fca7d7
symbols:
  - sf.substreams.foundational.store.v1.Store`,
			expected: DescriptorSetEntry{
				Module:  "buf.build/streamingfast/substreams-foundational-store",
				Version: "f3ab5976b9ba4f9bac28b26271fca7d7",
				Symbols: []string{"sf.substreams.foundational.store.v1.Store"},
			},
			expectError: false,
		},
		{
			name:        "error: buf commit reference too short (31 chars)",
			yaml:        `module: buf.build/streamingfast/substreams-sink-sql@f3ab5976b9ba4f9bac28b26271fca7d`,
			expectError: true,
			errorMsg:    "is not valid semantic version",
		},
		{
			name:        "error: buf commit reference too long (33 chars)",
			yaml:        `module: buf.build/streamingfast/substreams-sink-sql@f3ab5976b9ba4f9bac28b26271fca7d7a`,
			expectError: true,
			errorMsg:    "is not valid semantic version",
		},
		{
			name:        "error: buf commit reference with uppercase (invalid)",
			yaml:        `module: buf.build/streamingfast/substreams-sink-sql@F3AB5976B9BA4F9BAC28B26271FCA7D7`,
			expectError: true,
			errorMsg:    "is not valid semantic version",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var descriptorSetEntry DescriptorSetEntry
			err := yaml.Unmarshal([]byte(tt.yaml), &descriptorSetEntry)

			if tt.expectError {
				require.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expected.Module, descriptorSetEntry.Module)
			assert.Equal(t, tt.expected.Version, descriptorSetEntry.Version)
			assert.Equal(t, tt.expected.Symbols, descriptorSetEntry.Symbols)
		})
	}
}

func TestIsValidBufCommitRef(t *testing.T) {
	tests := []struct {
		version     string
		expected    bool
		description string
	}{
		{"f3ab5976b9ba4f9bac28b26271fca7d7", true, "valid 32 char lowercase hex"},
		{"0000000000000000000000000000000a", true, "valid hex with zeros"},
		{"abcdef0123456789abcdef0123456789", true, "valid hex with all valid chars"},
		{"", false, "empty string"},
		{"f3ab5976b9ba4f9bac28b26271fca7d", false, "31 chars (too short)"},
		{"f3ab5976b9ba4f9bac28b26271fca7d7a", false, "33 chars (too long)"},
		{"F3AB5976B9BA4F9BAC28B26271FCA7D7", false, "uppercase hex (invalid)"},
		{"g3ab5976b9ba4f9bac28b26271fca7d7", false, "invalid hex char 'g'"},
		{"v1.0.0", false, "semver (not a commit ref)"},
		{"main", false, "branch name"},
		{"latest", false, "latest keyword"},
	}

	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			result := isValidBufCommitRef(tt.version)
			assert.Equal(t, tt.expected, result, tt.description)
		})
	}
}
