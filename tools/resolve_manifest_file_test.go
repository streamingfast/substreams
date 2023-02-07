package tools

import (
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"os"
	"path/filepath"
	"testing"
)

func TestResolveManifestFile(t *testing.T) {
	type args struct {
		input       string
		dirToMake   string
		filesOnDisk []string
	}
	tests := []struct {
		name             string
		args             args
		wantManifestName string
		assertion        require.ErrorAssertionFunc
	}{
		{
			"no input provided",
			args{"", "", []string{"substreams.yaml"}},
			"substreams.yaml",
			require.NoError,
		},
		{
			"no input provided and not substreams.yaml present",
			args{"", "", []string{}},
			"",
			errorEqual("no manifest entered in directory without a manifest"),
		},
		{
			"input provided, valid manifest file",
			args{"substreams-custom.yaml", "", []string{"substreams-custom.yaml"}},
			"substreams-custom.yaml",
			require.NoError,
		},
		{
			"input provided, invalid manifest file",
			args{"substreams-custom.yaml", "", []string{}},
			"",
			errorEqual("read input file info: stat substreams-custom.yaml: no such file or directory"),
		},
		{
			"input provided, valid dir",
			args{"manifests-dir", "manifests-dir", []string{"substreams.yaml"}},
			"manifests-dir/substreams.yaml",
			require.NoError,
		},
		{
			"input provided, invalid dir",
			args{"manifests-dir", "manifests-dir", []string{}},
			"",
			errorEqual("read input file info: stat manifests-dir: no such file or directory"),
		},
		{
			"input provided, valid spkg",
			args{"https://github.com/org/repo/file.spkg", "", []string{}},
			"https://github.com/org/repo/file.spkg",
			require.NoError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), tt.args.dirToMake)

			for _, fileOnDisk := range tt.args.filesOnDisk {
				directory := filepath.Join(root, filepath.Dir(fileOnDisk))
				filename := filepath.Join(directory, filepath.Base(fileOnDisk))

				err := os.MkdirAll(directory, os.ModePerm)
				require.NoError(t, err)

				err = os.WriteFile(filename, []byte{}, os.ModePerm)
				require.NoError(t, err)
			}

			cwd, err := os.Getwd()
			require.NoError(t, err)

			defer func() {
				err := os.Chdir(cwd)
				require.NoError(t, err)
			}()

			if tt.args.dirToMake != "" {
				root = filepath.Dir(root)
			}
			err = os.Chdir(root)
			require.NoError(t, err)

			gotManifestName, err := ResolveManifestFile(tt.args.input)
			tt.assertion(t, err)
			assert.Equal(t, tt.wantManifestName, gotManifestName)
		})
	}
}

func errorEqual(expectedErrString string) require.ErrorAssertionFunc {
	return func(t require.TestingT, err error, msgAndArgs ...interface{}) {
		require.EqualError(t, err, expectedErrString, msgAndArgs...)
	}
}
