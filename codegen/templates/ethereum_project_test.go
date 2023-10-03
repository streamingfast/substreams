package templates

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/streamingfast/eth-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnsureOurProjectCompiles(t *testing.T) {
	abiContent, err := os.ReadFile("./ethereum/abi/contract.abi.json")
	require.NoError(t, err)

	abi, err := eth.ParseABIFromBytes(abiContent)
	require.NoError(t, err)

	project, err := NewEthereumProject("substreams-tests", "substreams_tests", EthereumChainsByID["Mainnet"], eth.MustNewAddress("0xbc4ca0eda7647a8ab7c2061c2e118a18a936f13d"), abi, string(abiContent), 123)
	require.NoError(t, err)

	files, err := project.Render()
	require.NoError(t, err)

	for _, fileToWrite := range []string{"src/lib.rs"} {
		content, found := files[fileToWrite]
		require.True(t, found)

		err = os.WriteFile(filepath.Join("ethereum", fileToWrite), content, os.ModePerm)
		require.NoError(t, err)
	}

	projectDir, err := filepath.Abs("./ethereum")
	require.NoError(t, err)

	cmd := exec.Command("cargo", "build", "--release", "--target", "wasm32-unknown-unknown")
	cmd.Dir = projectDir

	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "Command %q in %q failed with state %s\n%s", cmd, projectDir, cmd.ProcessState, string(output))
}

func TestNewEthereumTemplateProject(t *testing.T) {
	abi := fileContent(t, "ethereum/abi/contract.abi.json")

	type args struct {
		address string
		abi     []byte
	}
	tests := []struct {
		name      string
		args      args
		want      map[string][]byte
		assertion require.ErrorAssertionFunc
	}{
		{
			"standard case",
			args{"0xbc4ca0eda7647a8ab7c2061c2e118a18a936f13d", abi},
			map[string][]byte{
				"abi/contract.abi.json": abi,
				"proto/contract.proto":  fileContent(t, "./ethereum/proto/contract.proto"),
				"src/abi/mod.rs":        fileContent(t, "./ethereum/src/abi/mod.rs"),
				"src/pb/contract.v1.rs": fileContent(t, "./ethereum/src/pb/contract.v1.rs"),
				"src/pb/mod.rs":         fileContent(t, "./ethereum/src/pb/mod.rs"),
				"src/lib.rs":            fileContent(t, "./ethereum/src/lib.rs"),
				"build.rs":              fileContent(t, "./ethereum/build.rs"),
				"Cargo.lock":            fileContent(t, "./ethereum/Cargo.lock"),
				"Cargo.toml":            fileContent(t, "./ethereum/Cargo.toml"),
				"Makefile":              fileContent(t, "./ethereum/Makefile"),
				"substreams.yaml":       fileContent(t, "./ethereum/substreams.yaml"),
				"rust-toolchain.toml":   fileContent(t, "./ethereum/rust-toolchain.toml"),
				"schema.sql":            fileContent(t, "./ethereum/schema.sql"),
			},
			require.NoError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			abi, err := eth.ParseABIFromBytes(tt.args.abi)
			require.NoError(t, err)

			chain := EthereumChainsByID["Mainnet"]

			project, err := NewEthereumProject("substreams-init-test", "substreams_init_test", chain, eth.MustNewAddress("0xbc4ca0eda7647a8ab7c2061c2e118a18a936f13d"), abi, string(tt.args.abi), 123)
			require.NoError(t, err)

			got, err := project.Render()
			require.NoError(t, err)

			keysExpected := keys(tt.want)
			keysActual := keys(got)

			require.ElementsMatch(t, keysExpected, keysActual, "Entries key are different")
			for wantEntry, wantContent := range tt.want {
				filename := strings.ReplaceAll(wantEntry, string(filepath.Separator), "_")
				wantFilename := filepath.Join(os.TempDir(), fmt.Sprintf("want.%s", filename))
				gotFilename := filepath.Join(os.TempDir(), fmt.Sprintf("got.%s", filename))

				if !assert.Equal(t, string(wantContent), string(got[wantEntry]), "File %q amd %q are different", wantFilename, gotFilename) {
					err := os.WriteFile(wantFilename, wantContent, os.ModePerm)
					require.NoError(t, err)

					err = os.WriteFile(gotFilename, got[wantEntry], os.ModePerm)
					require.NoError(t, err)
				}
			}
		})
	}
}

func fileContent(t *testing.T, path string) []byte {
	content, err := os.ReadFile(path)
	require.NoError(t, err)

	return content
}
