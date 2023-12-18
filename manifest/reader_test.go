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

func TestMergeNetwork(t *testing.T) {

	tests := []struct {
		name              string
		srcNetwork        *pbsubstreams.NetworkParams
		destNetwork       *pbsubstreams.NetworkParams
		expectDestNetwork *pbsubstreams.NetworkParams
		expectPanic       bool
	}{
		{
			name:              "some-nil",
			srcNetwork:        &pbsubstreams.NetworkParams{},
			destNetwork:       nil,
			expectDestNetwork: nil,
			expectPanic:       true,
		},
		{
			name:       "nil-some",
			srcNetwork: nil,
			destNetwork: &pbsubstreams.NetworkParams{
				InitialBlocks: map[string]uint64{
					"mod1": 10,
				},
				Params: map[string]string{
					"mod1": "mod=1",
				},
			},
			expectDestNetwork: &pbsubstreams.NetworkParams{
				InitialBlocks: map[string]uint64{
					"mod1": 10,
				},
				Params: map[string]string{
					"mod1": "mod=1",
				},
			},
		},

		{
			name: "just append",
			srcNetwork: &pbsubstreams.NetworkParams{
				InitialBlocks: map[string]uint64{
					"mod1": 10,
					"mod2": 20,
				},
			},
			destNetwork: &pbsubstreams.NetworkParams{
				InitialBlocks: map[string]uint64{
					"mod2": 22,
					"mod3": 33,
				},
			},
			expectDestNetwork: &pbsubstreams.NetworkParams{
				InitialBlocks: map[string]uint64{
					"src:mod1": 10,
					"src:mod2": 20,
					"mod2":     22,
					"mod3":     33,
				},
			},
		},
		{
			name: "overwrite mod2",
			srcNetwork: &pbsubstreams.NetworkParams{
				InitialBlocks: map[string]uint64{
					"mod1": 10,
					"mod2": 20,
				},
				Params: map[string]string{
					"mod2": "mod=2",
				},
			},
			destNetwork: &pbsubstreams.NetworkParams{
				InitialBlocks: map[string]uint64{
					"src:mod2": 22,
					"mod3":     33,
				},
				Params: map[string]string{
					"mod2": "mod=22",
				},
			},
			expectDestNetwork: &pbsubstreams.NetworkParams{
				InitialBlocks: map[string]uint64{
					"src:mod1": 10,
					"src:mod2": 22,
					"mod3":     33,
				},
				Params: map[string]string{
					"src:mod2": "mod=2",
					"mod2":     "mod=22",
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.expectPanic {
				defer func() {
					if r := recover(); r == nil {
						t.Errorf("The code did not panic")
					}
				}()
			}
			mergeNetwork(test.srcNetwork, test.destNetwork, "src")
			assert.Equal(t, test.expectDestNetwork, test.destNetwork)

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
			expectDestNetworks: map[string]*pbsubstreams.NetworkParams{
				"sepolia": sepoliaPrefixed,
				"mainnet": mainnetPrefixed,
			},
		},
		{
			// case: src [mainnet,sepolia] dest [mainnet,sepolia] -> [mainnet,sepolia]
			name: "same+same=same",
			srcNetworks: map[string]*pbsubstreams.NetworkParams{
				"mainnet": {},
				"sepolia": {},
			},
			destNetworks: map[string]*pbsubstreams.NetworkParams{
				"mainnet": mainnet,
				"sepolia": sepolia,
			},
			expectDestNetworks: map[string]*pbsubstreams.NetworkParams{
				"mainnet": mainnet,
				"sepolia": sepolia,
			},
		},

		{
			// case: src [mainnet] dest [sepolia] -> [mainnetPrefixed, sepolia]
			name: "dest-is-different",
			srcNetworks: map[string]*pbsubstreams.NetworkParams{
				"mainnet": mainnet,
			},
			destNetworks: map[string]*pbsubstreams.NetworkParams{
				"sepolia": sepolia,
			},
			expectDestNetworks: map[string]*pbsubstreams.NetworkParams{
				"mainnet": mainnetPrefixed,
				"sepolia": sepolia,
			},
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
			mergeNetworks(src, dest, "src")
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

func Test_validateNetworks(t *testing.T) {
	mainnetString := "mainnet"
	tests := []struct {
		name                   string
		pkg                    *pbsubstreams.Package
		includeImportedModules map[string]bool
		overrideNetwork        *string
		wantErr                bool
	}{
		{
			name: "valid",
			pkg: &pbsubstreams.Package{
				Networks: map[string]*pbsubstreams.NetworkParams{
					"mainnet": {
						InitialBlocks: map[string]uint64{
							"mod1": 10,
						},
						Params: map[string]string{
							"mod1": "mod=1",
						},
					},
					"testnet": {
						InitialBlocks: map[string]uint64{
							"mod1": 20,
						},
						Params: map[string]string{
							"mod1": "mod=2",
						},
					},
				},
			},
		},
		{
			name: "invalid",
			pkg: &pbsubstreams.Package{
				Networks: map[string]*pbsubstreams.NetworkParams{
					"mainnet": {
						InitialBlocks: map[string]uint64{
							"mod1": 10,
						},
						Params: map[string]string{
							"mod1": "mod=1",
						},
					},
					"testnet": {
						InitialBlocks: map[string]uint64{
							"mod2": 20,
						},
						Params: map[string]string{
							"mod3": "mod=3",
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "valid for modules declared by dependencies",
			pkg: &pbsubstreams.Package{
				Networks: map[string]*pbsubstreams.NetworkParams{
					"mainnet": {
						InitialBlocks: map[string]uint64{
							"mod1": 10,
						},
						Params: map[string]string{
							"mod1":     "mod=1",
							"lib:mod3": "mod=3",
						},
					},
					"testnet": {
						InitialBlocks: map[string]uint64{
							"mod1": 20,
						},
						Params: map[string]string{
							"mod1": "mod=11",
						},
					},
				},
			},
		},
		{
			name: "invalid for modules declared by dependencies when those modules are in the map of modules to include",
			pkg: &pbsubstreams.Package{
				Networks: map[string]*pbsubstreams.NetworkParams{
					"mainnet": {
						InitialBlocks: map[string]uint64{
							"mod1": 10,
						},
						Params: map[string]string{
							"mod1":     "mod=1",
							"lib:mod3": "mod=3",
						},
					},
					"testnet": {
						InitialBlocks: map[string]uint64{
							"mod1": 20,
						},
						Params: map[string]string{
							"mod1": "mod=11",
						},
					},
				},
			},
			includeImportedModules: map[string]bool{
				"lib:mod3": true,
			},
			wantErr: true,
		},

		{
			name: "valid for whole networks declared by dependencies",
			pkg: &pbsubstreams.Package{
				Networks: map[string]*pbsubstreams.NetworkParams{
					"mainnet": {
						InitialBlocks: map[string]uint64{
							"mod1":     10,
							"lib:mod2": 10,
						},
						Params: map[string]string{
							"mod1":     "mod=1",
							"lib:mod3": "mod=3",
						},
					},
					"testnet": {
						InitialBlocks: map[string]uint64{
							"lib:mod2": 20,
						},
						Params: map[string]string{
							"lib:mod3": "mod=3",
						},
					},
				},
			},
		},
		{
			// even if we depend on an imported module, we don't have to support all its networks
			name: "still valid for whole networks declared by dependencies even if they are in the list of modules to include",
			pkg: &pbsubstreams.Package{
				Networks: map[string]*pbsubstreams.NetworkParams{
					"mainnet": {
						InitialBlocks: map[string]uint64{
							"mod1":     10,
							"lib:mod2": 10,
						},
						Params: map[string]string{
							"mod1":     "mod=1",
							"lib:mod3": "mod=3",
						},
					},
					"testnet": {
						InitialBlocks: map[string]uint64{
							"lib:mod2": 20,
						},
						Params: map[string]string{
							"lib:mod3": "mod=3",
						},
					},
				},
			},
			includeImportedModules: map[string]bool{
				"lib:mod3": true,
			},
		},
		{
			name: "invalid for networks declared by dependencies that would be selected as default network",
			pkg: &pbsubstreams.Package{
				Network: "testnet",
				Networks: map[string]*pbsubstreams.NetworkParams{
					"mainnet": {
						InitialBlocks: map[string]uint64{
							"mod1": 10,
						},
						Params: map[string]string{
							"mod1": "mod=1",
						},
					},
					"testnet": {
						InitialBlocks: map[string]uint64{
							"lib:mod2": 20,
						},
						Params: map[string]string{
							"lib:mod3": "mod=3",
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "network override works",
			pkg: &pbsubstreams.Package{
				Network: "testnet",
				Networks: map[string]*pbsubstreams.NetworkParams{
					"mainnet": {
						InitialBlocks: map[string]uint64{
							"mod1": 10,
						},
					},
				},
			},
			overrideNetwork: &mainnetString,
		},
		{
			name: "invalid for empty networks declared by dependencies that would be selected as default network",
			pkg: &pbsubstreams.Package{
				Network: "unavailable",
				Networks: map[string]*pbsubstreams.NetworkParams{
					"mainnet": {
						InitialBlocks: map[string]uint64{
							"mod1": 10,
						},
						Params: map[string]string{
							"mod1": "mod=1",
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "invalid for empty networks declared by dependencies that would be selected as default network",
			pkg: &pbsubstreams.Package{
				Network: "unavailable",
				Networks: map[string]*pbsubstreams.NetworkParams{
					"mainnet": {
						InitialBlocks: map[string]uint64{
							"mod1": 10,
						},
						Params: map[string]string{
							"mod1": "mod=1",
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "valid for empty networks when a network is selected",
			pkg: &pbsubstreams.Package{
				Network:  "unavailable",
				Networks: nil,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := validateNetworks(tt.pkg, tt.includeImportedModules, tt.overrideNetwork); (err != nil) != tt.wantErr {
				t.Errorf("validateNetworks() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func Test_dependentImportedModules(t *testing.T) {

	storeKind := &pbsubstreams.Module_KindStore_{KindStore: &pbsubstreams.Module_KindStore{}}
	mapKind := &pbsubstreams.Module_KindMap_{KindMap: &pbsubstreams.Module_KindMap{}}

	testModules := []*pbsubstreams.Module{
		{
			Name: "lib:storemod",
			Kind: storeKind,
		},
		{
			Name: "lib:mapmod",
			Kind: mapKind,
		},
		{
			Name: "mod_dep_on_mapmod",
			Kind: storeKind,
			Inputs: []*pbsubstreams.Module_Input{
				{
					Input: &pbsubstreams.Module_Input_Map_{Map: &pbsubstreams.Module_Input_Map{
						ModuleName: "lib:mapmod",
					}},
				},
			},
		},
		{
			Name: "mod_dep_on_two_mods",
			Kind: mapKind,
			Inputs: []*pbsubstreams.Module_Input{
				{
					Input: &pbsubstreams.Module_Input_Store_{Store: &pbsubstreams.Module_Input_Store{
						ModuleName: "lib:storemod",
					}},
				},
				{
					Input: &pbsubstreams.Module_Input_Map_{Map: &pbsubstreams.Module_Input_Map{
						ModuleName: "lib:mapmod",
					}},
				},
			},
		},
		{
			Name: "mod_independant",
			Kind: &pbsubstreams.Module_KindMap_{KindMap: &pbsubstreams.Module_KindMap{}},
		},
	}

	type args struct {
		graph        *ModuleGraph
		outputModule string
	}

	tests := []struct {
		name    string
		args    args
		want    map[string]bool
		wantErr bool
	}{
		{
			name: "independant",
			args: args{
				graph:        MustNewModuleGraph(testModules),
				outputModule: "mod_independant",
			},
			want: map[string]bool{},
		},
		{
			name: "dep_on_map",
			args: args{
				graph:        MustNewModuleGraph(testModules),
				outputModule: "mod_dep_on_mapmod",
			},
			want: map[string]bool{
				"lib:mapmod": true,
			},
		},
		{
			name: "dep_on_two",
			args: args{
				graph:        MustNewModuleGraph(testModules),
				outputModule: "mod_dep_on_two_mods",
			},
			want: map[string]bool{
				"lib:mapmod":   true,
				"lib:storemod": true,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := dependentImportedModules(tt.args.graph, &tt.args.outputModule)
			if (err != nil) != tt.wantErr {
				t.Errorf("dependentImportedModules() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			assert.Equalf(t, tt.want, got, "dependentImportedModules() = %v (nil=%t), want %v (nil=%t)", got, got == nil, tt.want, tt.want == nil)
		})
	}
}
