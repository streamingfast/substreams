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
		params           map[string]string
		workingDirectory string
	}

	tests := []struct {
		name                string
		args                args
		want                *pbsubstreams.Package
		assertionNew        require.ErrorAssertionFunc
		assertionRead       require.ErrorAssertionFunc
		skipProtoValidation bool
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
			false,
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
			false,
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
			false,
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
			false,
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
						newTestModuleModel("test_mapper", 0, "sf.test.Block", "proto:sf.test.Output"),
					},
				},
			},
			require.NoError,
			require.NoError,
			false,
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
			true,
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
			false,
		},
		{
			"imports_registry_notation.yaml",
			args{
				env: map[string]string{
					"SUBSTREAMS_REGISTRY": remoteServer.URL,
				},
			},
			&pbsubstreams.Package{
				Version:    1,
				ProtoFiles: readSystemProtoDescriptors(t),
				Modules:    &pbsubstreams.Modules{},
				PackageMeta: []*pbsubstreams.PackageMetadata{
					{Name: "test", Version: "v0.0.0"},
					{Name: "spkg1", Version: "v0.0.0"},
				},
			},
			require.NoError,
			require.NoError,
			true,
		},
		{
			"imports_too_many_at.yaml",
			args{
				env: map[string]string{"SUBSTREAMS_REGISTRY": remoteServer.URL},
			},
			nil,
			require.NoError,
			require.Error,
			false,
		},
		{
			"imports_invalid_semver.yaml",
			args{
				env: map[string]string{"SUBSTREAMS_REGISTRY": remoteServer.URL},
			},
			nil,
			require.NoError,
			require.Error,
			false,
		},
		{
			"imports_latest.yaml",
			args{
				env: map[string]string{"SUBSTREAMS_REGISTRY": remoteServer.URL},
			},
			nil,
			require.NoError,
			require.Error,
			true,
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
			false,
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
			false,
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
			false,
		},
		{
			"invalid_map_module.yaml",
			args{},
			nil,
			require.NoError,
			require.Error,
			false,
		},
		{
			"invalid_unknown_field.yaml",
			args{},
			nil,
			require.NoError,
			require.Error,
			false,
		},
		{
			"networks.yaml",
			args{},
			&pbsubstreams.Package{
				Version:    1,
				ProtoFiles: readSystemProtoDescriptors(t),
				PackageMeta: []*pbsubstreams.PackageMetadata{
					{
						Name:    "testnetworks",
						Version: "v0.1.0",
					},
				},
				ModuleMeta: []*pbsubstreams.ModuleMetadata{
					{},
					{},
				},
				Modules: &pbsubstreams.Modules{
					Binaries: []*pbsubstreams.Binary{newTestBinaryModel([]byte{})},
					Modules: []*pbsubstreams.Module{
						newTestModuleModel("mod1", 200, "sf.test.Block", "proto:sf.test.Output"),
						newTestModuleModel("mod2", 200, "map:mod1", "proto:sf.test.Output"),
					},
				},
				Network: "mainnet",
				Networks: map[string]*pbsubstreams.NetworkParams{
					"mainnet": {
						InitialBlocks: map[string]uint64{
							"mod1": 200,
						},
					},
					"sepolia": {
						InitialBlocks: map[string]uint64{
							"mod1": 400,
						},
					},
				},
			},
			require.NoError,
			require.NoError,
			false,
		},
		{
			"networks_with_params.yaml",
			args{},
			&pbsubstreams.Package{
				Version:    1,
				ProtoFiles: readSystemProtoDescriptors(t),
				PackageMeta: []*pbsubstreams.PackageMetadata{
					{
						Name:    "testnetworks",
						Version: "v0.1.0",
					},
				},
				ModuleMeta: []*pbsubstreams.ModuleMetadata{
					{},
				},
				Modules: &pbsubstreams.Modules{
					Binaries: []*pbsubstreams.Binary{newTestBinaryModel([]byte{})},
					Modules: []*pbsubstreams.Module{
						newTestModuleModel("mod1", 0, "params:val=toto", "proto:sf.test.Output"),
					},
				},
				Network: "mainnet",
				Networks: map[string]*pbsubstreams.NetworkParams{
					"mainnet": {
						Params: map[string]string{
							"mod1": "val=toto",
						},
					},
					"sepolia": {
						Params: map[string]string{
							"mod1": "val=tata",
						},
					},
				},
			},
			require.NoError,
			require.NoError,
			false,
		},
		{
			"networks_with_params.yaml",
			args{
				params: map[string]string{"mod1": "val=overloaded"},
			},
			&pbsubstreams.Package{
				Version:    1,
				ProtoFiles: readSystemProtoDescriptors(t),
				PackageMeta: []*pbsubstreams.PackageMetadata{
					{
						Name:    "testnetworks",
						Version: "v0.1.0",
					},
				},
				ModuleMeta: []*pbsubstreams.ModuleMetadata{
					{},
				},
				Modules: &pbsubstreams.Modules{
					Binaries: []*pbsubstreams.Binary{newTestBinaryModel([]byte{})},
					Modules: []*pbsubstreams.Module{
						newTestModuleModel("mod1", 0, "params:val=overloaded", "proto:sf.test.Output"),
					},
				},
				Network: "mainnet",
				Networks: map[string]*pbsubstreams.NetworkParams{
					"mainnet": {
						Params: map[string]string{
							"mod1": "val=toto",
						},
					},
					"sepolia": {
						Params: map[string]string{
							"mod1": "val=tata",
						},
					},
				},
			},
			require.NoError,
			require.NoError,
			false,
		},

		{
			"networks_missing_default.yaml",
			args{},
			nil,
			require.NoError,
			require.Error,
			false,
		},
		{
			"networks_inconsistent.yaml",
			args{},
			nil,
			require.NoError,
			require.Error,
			false,
		},
		{
			"with-exclude-paths.yaml",
			args{},
			nil,
			require.NoError,
			require.NoError,
			true,
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

			if tt.args.params != nil {
				readerOptions = append(readerOptions, WithParams(tt.args.params))
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

			pkgBundle, err := r.Read()
			tt.assertionRead(t, err)

			if pkgBundle != nil {
				assertProtoEqual(t, tt.want, pkgBundle.Package, tt.skipProtoValidation)
			}
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
	var input *pbsubstreams.Module_Input
	if strings.HasPrefix(inputType, "map:") {
		input = &pbsubstreams.Module_Input{
			Input: &pbsubstreams.Module_Input_Map_{Map: &pbsubstreams.Module_Input_Map{
				ModuleName: strings.TrimPrefix(inputType, "map:"),
			},
			},
		}
	} else if strings.HasPrefix(inputType, "params:") {
		input = &pbsubstreams.Module_Input{
			Input: &pbsubstreams.Module_Input_Params_{Params: &pbsubstreams.Module_Input_Params{
				Value: strings.TrimPrefix(inputType, "params:"),
			},
			},
		}
	} else {
		input = &pbsubstreams.Module_Input{
			Input: &pbsubstreams.Module_Input_Source_{
				Source: &pbsubstreams.Module_Input_Source{
					Type: inputType,
				},
			},
		}
	}

	return &pbsubstreams.Module{
		Name:             name,
		BinaryEntrypoint: name,
		InitialBlock:     initialBlock,
		Kind: &pbsubstreams.Module_KindMap_{
			KindMap: &pbsubstreams.Module_KindMap{
				OutputType: outputType,
			},
		},
		Inputs: []*pbsubstreams.Module_Input{
			input,
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
				graph:        mustNewModuleGraph(testModules),
				outputModule: "mod_independant",
			},
			want: map[string]bool{},
		},
		{
			name: "dep_on_map",
			args: args{
				graph:        mustNewModuleGraph(testModules),
				outputModule: "mod_dep_on_mapmod",
			},
			want: map[string]bool{
				"lib:mapmod": true,
			},
		},
		{
			name: "dep_on_two",
			args: args{
				graph:        mustNewModuleGraph(testModules),
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
			got, err := dependentImportedModules(tt.args.graph, tt.args.outputModule)
			if (err != nil) != tt.wantErr {
				t.Errorf("dependentImportedModules() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			assert.Equalf(t, tt.want, got, "dependentImportedModules() = %v (nil=%t), want %v (nil=%t)", got, got == nil, tt.want, tt.want == nil)
		})
	}
}

func mustNewModuleGraph(modules []*pbsubstreams.Module) *ModuleGraph {
	g, err := NewModuleGraph(modules)
	if err != nil {
		panic(err)
	}
	return g
}

func TestReader_RegistryErrorHandling(t *testing.T) {
	tests := []struct {
		name          string
		statusCode    int
		registryURL   string
		expectedError string
	}{
		{
			name:          "package not found",
			statusCode:    http.StatusNotFound,
			registryURL:   "https://spkg.io",
			expectedError: "package does not exist on the Substreams registry",
		},
		{
			name:          "access denied",
			statusCode:    http.StatusForbidden,
			registryURL:   "https://spkg.io",
			expectedError: "access denied to package on the Substreams registry",
		},
		{
			name:          "server error",
			statusCode:    http.StatusInternalServerError,
			registryURL:   "https://spkg.io",
			expectedError: "Substreams package registry is temporarily unavailable (status 500 Internal Server Error)",
		},
		{
			name:          "bad gateway",
			statusCode:    http.StatusBadGateway,
			registryURL:   "https://spkg.io",
			expectedError: "Substreams package registry is temporarily unavailable (status 502 Bad Gateway)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a test server that returns the specified status code
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
			}))
			defer server.Close()

			// Create a reader with the test server as the registry URL
			reader := &Reader{
				registryURL: server.URL,
			}

			// Construct the registry URL for the package
			packageURL := server.URL + "/v1/packages/common/0.1.0"

			// Test the readFromHttp method
			err := reader.readFromHttp(packageURL)

			// Verify the error message
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.expectedError)
		})
	}
}

func TestReader_IsRegistryURL(t *testing.T) {
	tests := []struct {
		name        string
		registryURL string
		testURL     string
		expected    bool
	}{
		{
			name:        "valid registry URL",
			registryURL: "https://spkg.io",
			testURL:     "https://spkg.io/v1/packages/common/0.1.0",
			expected:    true,
		},
		{
			name:        "non-registry URL",
			registryURL: "https://spkg.io",
			testURL:     "https://github.com/streamingfast/substreams/releases/download/v1.0.0/package.spkg",
			expected:    false,
		},
		{
			name:        "custom registry URL",
			registryURL: "https://custom-registry.com",
			testURL:     "https://custom-registry.com/v1/packages/test/1.0.0",
			expected:    true,
		},
		{
			name:        "registry base URL match",
			registryURL: "https://spkg.io",
			testURL:     "https://spkg.io/some/other/path",
			expected:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := &Reader{
				registryURL: tt.registryURL,
			}

			result := reader.isRegistryURL(tt.testURL)
			assert.Equal(t, tt.expected, result)
		})
	}
}
