package manifest

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jhump/protoreflect/desc/protoparse"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/descriptorpb"
)

func TestReader_ReadWithOverride(t *testing.T) {
	path, err := filepath.Abs("testdata/")
	require.NoError(t, err)

	override, err := filepath.Abs("testdata/with-params-override.yaml")

	r, err := newReader(override, path)
	require.NoError(t, err)

	pkg, err := r.Read()
	require.NoError(t, err)
	require.NotNil(t, pkg)
}

func TestReader_ReadWithNestedOverride(t *testing.T) {
	path, err := filepath.Abs("testdata/")
	require.NoError(t, err)

	override, err := filepath.Abs("testdata/with-params-override-override.yaml")
	require.NoError(t, err)

	r, err := newReader(override, path)
	require.NoError(t, err)

	pkg, err := r.Read()
	require.NoError(t, err)
	require.NotNil(t, pkg)
}

func TestReader_ReadWithRemoteOverride(t *testing.T) {
	path, err := filepath.Abs("testdata/")
	require.NoError(t, err)

	override, err := filepath.Abs("testdata/univ3-override.yaml")
	require.NoError(t, err)

	r, err := newReader(override, path)
	require.NoError(t, err)

	_, err = r.Read()
	require.NoError(t, err)
}

func TestReader_Read(t *testing.T) {
	absolutePathToInferredManifest, err := filepath.Abs("testdata/inferred_manifest")
	require.NoError(t, err)

	absolutePathToDep2, err := filepath.Abs("testdata/dep2.yaml")
	require.NoError(t, err)

	absolutePathToProto2, err := filepath.Abs("testdata/proto2")
	require.NoError(t, err)

	spkg1Content, err := os.ReadFile("testdata/spkg1/spkg1-v0.0.0.spkg")
	require.NoError(t, err)

	remoteServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write(spkg1Content)
	}))
	defer remoteServer.Close()

	type args struct {
		// If nil, the input is taken from the name
		input            *string
		env              map[string]string
		validateBinary   bool
		workingDirectory string
	}

	tests := []struct {
		name          string
		args          args
		want          *pbsubstreams.Package
		assertionNew  require.ErrorAssertionFunc
		assertionRead require.ErrorAssertionFunc
	}{
		{
			"bare_minimum.yaml",
			args{},
			&pbsubstreams.Package{
				Version:    1,
				ProtoFiles: readSystemProtoDescriptors(t),
				Modules:    &pbsubstreams.Modules{},
				PackageMeta: []*pbsubstreams.PackageMetadata{
					{
						Name:    "test",
						Version: "v0.0.0",
					},
				},
			},
			require.NoError,
			require.NoError,
		},
		{
			"from_folder",
			args{},
			&pbsubstreams.Package{
				Version:    1,
				ProtoFiles: readSystemProtoDescriptors(t),
				Modules:    &pbsubstreams.Modules{},
				PackageMeta: []*pbsubstreams.PackageMetadata{
					{
						Name:    "test",
						Version: "v0.0.0",
					},
				},
			},
			require.NoError,
			require.NoError,
		},
		{
			"empty_input",
			args{
				input:            new(string),
				workingDirectory: absolutePathToInferredManifest,
			},
			&pbsubstreams.Package{
				Version:    1,
				ProtoFiles: readSystemProtoDescriptors(t),
				Modules:    &pbsubstreams.Modules{},
				PackageMeta: []*pbsubstreams.PackageMetadata{
					{
						Name:    "test",
						Version: "v0.0.0",
					},
				},
			},
			require.NoError,
			require.NoError,
		},
		{
			"imports_relative_path.yaml",
			args{},
			&pbsubstreams.Package{
				Version:    1,
				ProtoFiles: readSystemProtoDescriptors(t),
				Modules:    &pbsubstreams.Modules{},
				PackageMeta: []*pbsubstreams.PackageMetadata{
					{
						Name:    "test",
						Version: "v0.0.0",
					},
					{
						Name:    "dep1",
						Version: "v0.0.0",
					},
				},
			},
			require.NoError,
			require.NoError,
		},
		{
			"binaries_relative_path.yaml",
			args{validateBinary: true},
			&pbsubstreams.Package{
				Version:    1,
				ProtoFiles: readSystemProtoDescriptors(t),
				PackageMeta: []*pbsubstreams.PackageMetadata{
					{
						Name:    "test",
						Version: "v0.0.0",
					},
				},
				ModuleMeta: []*pbsubstreams.ModuleMetadata{
					{},
				},
				Modules: &pbsubstreams.Modules{
					Binaries: []*pbsubstreams.Binary{newTestBinaryModel([]byte{})},
					Modules: []*pbsubstreams.Module{
						newTestModuleModel("test_mapper", UNSET, "sf.test.Block", "proto:sf.test.Output"),
					},
				},
			},
			require.NoError,
			require.NoError,
		},
		{
			"imports_http_url.yaml",
			args{
				env: map[string]string{
					"SERVER_HOST": strings.Replace(remoteServer.URL, "http://", "", 1),
				},
			},
			&pbsubstreams.Package{
				Version:    1,
				ProtoFiles: readSystemProtoDescriptors(t),
				Modules:    &pbsubstreams.Modules{},
				PackageMeta: []*pbsubstreams.PackageMetadata{
					{
						Name:    "test",
						Version: "v0.0.0",
					},
					{
						Name:    "spkg1",
						Version: "v0.0.0",
					},
				},
			},
			require.NoError,
			require.NoError,
		},
		{
			"imports_expand_env_variables.yaml",
			args{
				env: map[string]string{
					"RELATIVE_PATH_TO_DEP1": "./dep1.yaml",
					"ABSOLUTE_PATH_TO_DEP2": absolutePathToDep2,
				},
			},
			&pbsubstreams.Package{
				Version:    1,
				ProtoFiles: readSystemProtoDescriptors(t),
				Modules:    &pbsubstreams.Modules{},
				PackageMeta: []*pbsubstreams.PackageMetadata{
					{
						Name:    "test",
						Version: "v0.0.0",
					},
					{
						Name:    "dep1",
						Version: "v0.0.0",
					},
					{
						Name:    "dep2",
						Version: "v0.0.0",
					},
				},
			},
			require.NoError,
			require.NoError,
		},
		{
			"protobuf_files_relative_path.yaml",
			args{},
			&pbsubstreams.Package{
				Version: 1,
				ProtoFiles: withSystemProtoDefs(t,
					readProtoDescriptor(t, "./testdata", "./proto1/sf/substreams/test1.proto"),
				),
				Modules: &pbsubstreams.Modules{},
				PackageMeta: []*pbsubstreams.PackageMetadata{
					{
						Name:    "test",
						Version: "v0.0.0",
					},
				},
			},
			require.NoError,
			require.NoError,
		},
		{
			"protobuf_importPaths_relative_path.yaml",
			args{},
			&pbsubstreams.Package{
				Version: 1,
				ProtoFiles: withSystemProtoDefs(t,
					readProtoDescriptor(t, "testdata/proto1", "sf/substreams/test1.proto"),
				),
				Modules: &pbsubstreams.Modules{},
				PackageMeta: []*pbsubstreams.PackageMetadata{
					{
						Name:    "test",
						Version: "v0.0.0",
					},
				},
			},
			require.NoError,
			require.NoError,
		},
		{
			"protobuf_importPaths_expand_variables.yaml",
			args{
				env: map[string]string{
					"RELATIVE_PATH_TO_PROTO1": "./proto1",
					"ABSOLUTE_PATH_TO_PROTO2": absolutePathToProto2,
				},
			},
			&pbsubstreams.Package{
				Version: 1,
				ProtoFiles: withSystemProtoDefs(t,
					readProtoDescriptor(t, "testdata/proto1", "sf/substreams/test1.proto"),
					readProtoDescriptor(t, "testdata/proto2", "sf/substreams/test2.proto"),
				),
				Modules: &pbsubstreams.Modules{},
				PackageMeta: []*pbsubstreams.PackageMetadata{
					{
						Name:    "test",
						Version: "v0.0.0",
					},
				},
			},
			require.NoError,
			require.NoError,
		},
		{
			"invalid_map_module.yaml",
			args{},
			nil,
			require.NoError,
			require.Error,
		},
		{
			"invalid_unknown_field.yaml",
			args{},
			nil,
			require.NoError,
			require.Error,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for envKey, envValue := range tt.args.env {
				t.Setenv(envKey, envValue)
			}

			var readerOptions []Option
			if !tt.args.validateBinary {
				readerOptions = append(readerOptions, SkipSourceCodeReader())
			}

			var manifestPath string
			if tt.args.input != nil {
				manifestPath = *tt.args.input
			} else {
				var err error
				manifestPath, err = filepath.Abs(filepath.Join("testdata", tt.name))
				require.NoError(t, err)
			}

			workingDir := ""
			if tt.args.workingDirectory != "" {
				workingDir = tt.args.workingDirectory
			}

			r, err := newReader(manifestPath, workingDir, readerOptions...)
			tt.assertionNew(t, err)

			got, err := r.Read()
			tt.assertionRead(t, err)
			assertProtoEqual(t, tt.want, got)
		})
	}
}

func TestMergeNetworks(t *testing.T) {

	var sepolia = &pbsubstreams.NetworkParams{
		InitialBlocks: map[string]uint64{
			"mod1": 10,
		},
		Params: map[string]string{
			"mod2": "addr=0xdeadbeef",
		},
	}
	var mainnet = &pbsubstreams.NetworkParams{
		InitialBlocks: map[string]uint64{
			"mod1": 20,
		},
		Params: map[string]string{
			"mod2": "addr=0x12121212",
		},
	}
	var sepoliaPrefixed = &pbsubstreams.NetworkParams{
		InitialBlocks: map[string]uint64{
			"src:mod1": 10,
		},
		Params: map[string]string{
			"src:mod2": "addr=0xdeadbeef",
		},
	}
	var mainnetPrefixed = &pbsubstreams.NetworkParams{
		InitialBlocks: map[string]uint64{
			"src:mod1": 20,
		},
		Params: map[string]string{
			"src:mod2": "addr=0x12121212",
		},
	}

	tests := []struct {
		name               string
		srcNetworks        map[string]*pbsubstreams.NetworkParams
		destNetworks       map[string]*pbsubstreams.NetworkParams
		expectError        string
		expectDestNetworks map[string]*pbsubstreams.NetworkParams
	}{
		{
			// src []  dest []  -> []
			name:               "nil-nil",
			srcNetworks:        nil,
			destNetworks:       nil,
			expectError:        "",
			expectDestNetworks: nil,
		},
		{
			// case: src []  dest [mainnet,sepolia] -> [mainnet,sepolia]
			name:        "nil+some=some",
			srcNetworks: nil,
			destNetworks: map[string]*pbsubstreams.NetworkParams{
				"sepolia": sepolia,
				"mainnet": mainnet,
			},
			expectDestNetworks: map[string]*pbsubstreams.NetworkParams{
				"sepolia": sepolia,
				"mainnet": mainnet,
			},
		},
		{
			// case: src [mainnet,sepolia] dest [] -> [mainnet,sepolia]
			name: "some+nil=prefixed",
			srcNetworks: map[string]*pbsubstreams.NetworkParams{
				"sepolia": sepolia,
				"mainnet": mainnet,
			},
			destNetworks: nil,
			expectError:  "",
			expectDestNetworks: map[string]*pbsubstreams.NetworkParams{
				"sepolia": sepoliaPrefixed,
				"mainnet": mainnetPrefixed,
			},
		},
		{
			// case: src [mainnet,sepolia] dest [mainnet,sepolia] -> [mainnet,sepolia]
			name: "same+same=merged_prefixed",
			srcNetworks: map[string]*pbsubstreams.NetworkParams{
				"mainnet": {
					InitialBlocks: map[string]uint64{
						"mod3": 100,
					},
					Params: map[string]string{
						"mod4": "addr=0xffffffff",
					},
				},
				"sepolia": {
					InitialBlocks: map[string]uint64{
						"mod3": 200,
					},
					Params: map[string]string{
						"mod4": "addr=0xbbbbbbbb",
					},
				},
			},
			destNetworks: map[string]*pbsubstreams.NetworkParams{
				"mainnet": mainnet,
				"sepolia": sepolia,
			},
			expectError: "",
			expectDestNetworks: map[string]*pbsubstreams.NetworkParams{
				"mainnet": {
					InitialBlocks: map[string]uint64{
						"mod1":     20,
						"src:mod3": 100,
					},
					Params: map[string]string{
						"mod2":     "addr=0x12121212",
						"src:mod4": "addr=0xffffffff",
					},
				},
				"sepolia": {
					InitialBlocks: map[string]uint64{
						"mod1":     10,
						"src:mod3": 200,
					},
					Params: map[string]string{
						"mod2":     "addr=0xdeadbeef",
						"src:mod4": "addr=0xbbbbbbbb",
					},
				},
			},
		},
		{
			// case: src [mainnet,sepolia] dest [mainnet] -> [mainnet]
			name: "dest-is-subset",
			srcNetworks: map[string]*pbsubstreams.NetworkParams{
				"mainnet": {},
				"sepolia": {},
			},
			destNetworks: map[string]*pbsubstreams.NetworkParams{
				"mainnet": {},
			},
			expectError: "",
			expectDestNetworks: map[string]*pbsubstreams.NetworkParams{
				"mainnet": {},
			},
		},
		{
			// case: src [mainnet]          dest [mainnet,sepolia] -> ERROR
			name: "missing-in-source",
			srcNetworks: map[string]*pbsubstreams.NetworkParams{
				"mainnet": {},
			},
			destNetworks: map[string]*pbsubstreams.NetworkParams{
				"mainnet": {},
				"sepolia": {},
			},
			expectError: `network "sepolia" defined in package "dest_package" but not in "source_package"`,
		},
		{
			// case: src [mainnet] dest [mainnet,sepolia] -> no error if src:sepolia is overloaded
			name: "missing-in-source-but-overloaded",
			srcNetworks: map[string]*pbsubstreams.NetworkParams{
				"mainnet": {},
			},
			destNetworks: map[string]*pbsubstreams.NetworkParams{
				"mainnet": {
					InitialBlocks: map[string]uint64{
						"src:mod1": 100,
					},
				},
				"sepolia": {},
			},
			expectDestNetworks: map[string]*pbsubstreams.NetworkParams{
				"mainnet": {
					InitialBlocks: map[string]uint64{
						"src:mod1": 100,
					},
				},
				"sepolia": {},
			},
		},
		{
			// case: src [mainnet, goerli]  dest [mainnet,sepolia] -> ERROR
			name: "different",
			srcNetworks: map[string]*pbsubstreams.NetworkParams{
				"mainnet": {},
				"goerli":  {},
			},
			destNetworks: map[string]*pbsubstreams.NetworkParams{
				"mainnet": {},
				"sepolia": {},
			},
			expectError: `network "sepolia" defined in package "dest_package" but not in "source_package"`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			src := &pbsubstreams.Package{
				PackageMeta: []*pbsubstreams.PackageMetadata{
					{
						Name: "source_package",
					},
				},
				Networks: test.srcNetworks,
			}
			dest := &pbsubstreams.Package{
				PackageMeta: []*pbsubstreams.PackageMetadata{
					{
						Name: "dest_package",
					},
				},
				Networks: test.destNetworks,
			}
			err := mergeNetworks(src, dest, "src")
			if test.expectError != "" {
				require.Equal(t, err.Error(), test.expectError)
				return
			}
			assert.Equal(t, test.expectDestNetworks, dest.Networks)
		})
	}
}

func newTestBinaryModel(content []byte) *pbsubstreams.Binary {
	return &pbsubstreams.Binary{
		Type:    "wasm/rust-v1",
		Content: content,
	}
}

func newTestModuleModel(name string, initialBlock uint64, inputType string, outputType string) *pbsubstreams.Module {
	return &pbsubstreams.Module{
		Name:             name,
		BinaryEntrypoint: name,
		InitialBlock:     18446744073709551615,
		Kind: &pbsubstreams.Module_KindMap_{
			KindMap: &pbsubstreams.Module_KindMap{
				OutputType: outputType,
			},
		},
		Inputs: []*pbsubstreams.Module_Input{
			{
				Input: &pbsubstreams.Module_Input_Source_{
					Source: &pbsubstreams.Module_Input_Source{
						Type: inputType,
					},
				},
			},
		},
		Output: &pbsubstreams.Module_Output{
			Type: outputType,
		},
	}
}

func readProtoDescriptor(t *testing.T, importPath string, file string) (out *descriptorpb.FileDescriptorProto) {
	t.Helper()

	parser := protoparse.Parser{
		ImportPaths:           []string{importPath},
		IncludeSourceCodeInfo: true,
	}

	customFiles, err := parser.ParseFiles(file)
	require.NoError(t, err)
	require.Len(t, customFiles, 1)

	return customFiles[0].AsFileDescriptorProto()
}

func withSystemProtoDefs(t *testing.T, additionalProto ...*descriptorpb.FileDescriptorProto) (out []*descriptorpb.FileDescriptorProto) {
	t.Helper()

	out = readSystemProtoDescriptors(t)
	out = append(out, additionalProto...)
	return
}

func readSystemProtoDescriptors(t *testing.T) (out []*descriptorpb.FileDescriptorProto) {
	t.Helper()

	systemProtoFiles, err := readSystemProtobufs()
	require.NoError(t, err)

	return systemProtoFiles.File
}
