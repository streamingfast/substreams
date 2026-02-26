package sink

import (
	"encoding/hex"
	"testing"

	"github.com/streamingfast/substreams/manifest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadManifestAndModule(t *testing.T) {
	type args struct {
		manifestPath             string
		params                   []string
		outputModuleName         string
		expectedOutputModuleType string
		skipPackageValudation    bool
	}
	tests := []struct {
		name                 string
		args                 args
		wantPackagePresent   bool
		wantModuleName       string
		wantOutputModuleHash string
		assertion            assert.ErrorAssertionFunc
	}{
		{
			"default",
			args{"testdata/substreams.yaml", nil, "kv_out", "kv-out", false},
			true,
			"kv_out",
			"f0b74c6dc57fa840bf1e7ff526431f9f1b5240d0",
			assert.NoError,
		},
		{
			"multile expected type accepted",
			args{"testdata/substreams.yaml", nil, "kv_out", "kv-out,graph-out", false},
			true,
			"kv_out",
			"f0b74c6dc57fa840bf1e7ff526431f9f1b5240d0",
			assert.NoError,
		},
		{
			"multile expected type accepted, inverted",
			args{"testdata/substreams.yaml", nil, "graph_out", "kv-out,graph-out", false},
			true,
			"graph_out",
			"54acb6611a4a4b430c81e66639159efb49b618d5",
			assert.NoError,
		},
		{
			"params a",
			args{"testdata/substreams.yaml", []string{"params_out=a"}, "params_out", "string", false},
			true,
			"params_out",
			"0f320848f35675facdd72fd383fcd0803fa87c42",
			assert.NoError,
		},
		{
			"params b",
			args{"testdata/substreams.yaml", []string{"params_out=b"}, "params_out", "string", false},
			true,
			"params_out",
			"d3c994c6dddfb3a38097e44e6056cd5452a0b95e",
			assert.NoError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotPkg, gotModule, gotOutputModuleHash, err := ReadManifestAndModule(tt.args.manifestPath, "", tt.args.params, tt.args.outputModuleName, tt.args.expectedOutputModuleType, tt.args.skipPackageValudation, nil, zlog)
			tt.assertion(t, err)

			if tt.wantPackagePresent {
				assert.NotNil(t, gotPkg)
			} else {
				assert.Nil(t, gotPkg)
			}

			assert.Equal(t, tt.wantModuleName, gotModule.Name)
			assert.Equal(t, tt.wantOutputModuleHash, hex.EncodeToString(gotOutputModuleHash))
		})
	}
}

func TestReadManifestAndModule_WithNetworks_ExplicitModule(t *testing.T) {
	// Test that reading a manifest with networks works when an explicit
	// module name is provided.
	gotPkg, gotModule, _, err := ReadManifestAndModule(
		"testdata/substreams_with_networks.yaml",
		"",      // network (use default from manifest)
		nil,     // params
		"kv_out", // explicit module name
		IgnoreOutputModuleType,
		false, // skipPackageValidation
		nil,   // additionalOptions
		zlog,
	)
	require.NoError(t, err)
	require.NotNil(t, gotPkg)
	assert.Equal(t, "kv_out", gotModule.Name)
}

func TestReadManifestAndModule_WithNetworks_WithOverrideOutputModule_SentinelValue(t *testing.T) {
	// This is a regression test that documents the bug where passing
	// WithOverrideOutputModule with the InferOutputModuleFromPackage sentinel
	// value would cause an error when the manifest has a networks: section.
	//
	// The bug occurred because the sentinel value was passed to the manifest reader
	// which tried to look up ancestors of the sentinel module name in the graph
	// during network validation (in dependentImportedModules).
	//
	// The fix in sinker_viper.go (ConfigFromViper) prevents the sentinel value
	// from being passed to WithOverrideOutputModule. This test documents what
	// happens if the sentinel value IS incorrectly passed.
	_, _, _, err := ReadManifestAndModule(
		"testdata/substreams_with_networks.yaml",
		"",
		nil,
		"kv_out", // Use explicit module name for the function parameter
		IgnoreOutputModuleType,
		false,
		[]manifest.Option{
			// This simulates what the bug was: the sentinel value being passed
			// to WithOverrideOutputModule in the additionalOptions
			manifest.WithOverrideOutputModule(InferOutputModuleFromPackage),
		},
		zlog,
	)

	// With the sentinel value passed to WithOverrideOutputModule, we expect an error
	// because the manifest reader tries to find the module "@!##_InferOutputModuleFromSpkg_##!@"
	// in the graph when processing the networks section.
	require.Error(t, err)
	assert.Contains(t, err.Error(), InferOutputModuleFromPackage)
}
