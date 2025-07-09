package sink

import (
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/assert"
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
			gotPkg, gotModule, gotOutputModuleHash, err := ReadManifestAndModule(tt.args.manifestPath, "", tt.args.params, tt.args.outputModuleName, tt.args.expectedOutputModuleType, tt.args.skipPackageValudation, zlog)
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
