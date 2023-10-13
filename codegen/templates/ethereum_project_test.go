package templates

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/streamingfast/substreams/codegen"

	"github.com/streamingfast/eth-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnsureOurProjectCompiles(t *testing.T) {
	abiContent, err := os.ReadFile("./ethereum/abi/contract.abi.json")
	require.NoError(t, err)

	abi, err := eth.ParseABIFromBytes(abiContent)
	require.NoError(t, err)

	ethereumContracts := []*EthereumContract{NewEthereumContract(
		"",
		eth.MustNewAddress("0xbc4ca0eda7647a8ab7c2061c2e118a18a936f13d"),
		nil,
		abi,
		string(abiContent),
	)}

	for _, contract := range ethereumContracts {
		events, err := BuildEventModels(contract, len(ethereumContracts) > 1)
		require.NoError(t, err)
		contract.SetEvents(events)
	}

	project, err := NewEthereumProject(
		"",
		"substreams_tests",
		EthereumChainsByID["Mainnet"],
		ethereumContracts,
		123,
		codegen.SinkChoiceDb,
	)
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
	abiContent := fileContent(t, "ethereum/abi/contract.abi.json")

	type args struct {
		address   string
		abi       []byte
		shortName string
	}
	tests := []struct {
		name      string
		args      []args
		want      map[string][]byte
		choice    codegen.SinkChoice
		assertion require.ErrorAssertionFunc
	}{
		{
			name: "standard case - no sink",
			args: []args{
				{
					address:   "0xbc4ca0eda7647a8ab7c2061c2e118a18a936f13d",
					abi:       abiContent,
					shortName: "",
				},
			},
			want: map[string][]byte{
				"abi/contract.abi.json": abiContent,
				"proto/contract.proto":  fileContent(t, "./ethereum/proto/contract.proto"),
				"src/abi/mod.rs":        fileContent(t, "./ethereum/src/abi/mod.rs"),
				"src/pb/mod.rs":         fileContent(t, "./ethereum/src/pb/mod.rs"),
				"src/lib.rs":            fileContent(t, "./ethereum/results/lib.rs"),
				"build.rs":              fileContent(t, "./ethereum/build.rs"),
				"Cargo.lock":            fileContent(t, "./ethereum/Cargo.lock"),
				"Cargo.toml":            fileContent(t, "./ethereum/results/Cargo.toml"),
				"Makefile":              fileContent(t, "./ethereum/Makefile"),
				"substreams.yaml":       fileContent(t, "./ethereum/results/nosink/substreams.yaml"),
				"rust-toolchain.toml":   fileContent(t, "./ethereum/rust-toolchain.toml"),
				"schema.sql":            fileContent(t, "./ethereum/schema.sql"),
				"schema.graphql":        fileContent(t, "./ethereum/schema.graphql"),
				"subgraph.yaml":         fileContent(t, "./ethereum/subgraph.yaml"),
			},
			choice:    codegen.SinkChoiceNo,
			assertion: require.NoError,
		},
		{
			name: "standard case - db sink",
			args: []args{
				{
					address:   "0xbc4ca0eda7647a8ab7c2061c2e118a18a936f13d",
					abi:       abiContent,
					shortName: "",
				},
			},
			want: map[string][]byte{
				"abi/contract.abi.json": abiContent,
				"proto/contract.proto":  fileContent(t, "./ethereum/proto/contract.proto"),
				"src/abi/mod.rs":        fileContent(t, "./ethereum/src/abi/mod.rs"),
				"src/pb/mod.rs":         fileContent(t, "./ethereum/src/pb/mod.rs"),
				"src/lib.rs":            fileContent(t, "./ethereum/results/lib.rs"),
				"build.rs":              fileContent(t, "./ethereum/build.rs"),
				"Cargo.lock":            fileContent(t, "./ethereum/Cargo.lock"),
				"Cargo.toml":            fileContent(t, "./ethereum/results/Cargo.toml"),
				"Makefile":              fileContent(t, "./ethereum/Makefile"),
				"substreams.yaml":       fileContent(t, "./ethereum/results/dbsink/substreams.yaml"),
				"rust-toolchain.toml":   fileContent(t, "./ethereum/rust-toolchain.toml"),
				"schema.sql":            fileContent(t, "./ethereum/schema.sql"),
				"schema.graphql":        fileContent(t, "./ethereum/schema.graphql"),
				"subgraph.yaml":         fileContent(t, "./ethereum/subgraph.yaml"),
			},
			choice:    codegen.SinkChoiceDb,
			assertion: require.NoError,
		},
		{
			name: "standard case - graph sink",
			args: []args{
				{
					address:   "0xbc4ca0eda7647a8ab7c2061c2e118a18a936f13d",
					abi:       abiContent,
					shortName: "",
				},
			},
			want: map[string][]byte{
				"abi/contract.abi.json": abiContent,
				"proto/contract.proto":  fileContent(t, "./ethereum/proto/contract.proto"),
				"src/abi/mod.rs":        fileContent(t, "./ethereum/src/abi/mod.rs"),
				"src/pb/mod.rs":         fileContent(t, "./ethereum/src/pb/mod.rs"),
				"src/lib.rs":            fileContent(t, "./ethereum/results/lib.rs"),
				"build.rs":              fileContent(t, "./ethereum/build.rs"),
				"Cargo.lock":            fileContent(t, "./ethereum/Cargo.lock"),
				"Cargo.toml":            fileContent(t, "./ethereum/results/Cargo.toml"),
				"Makefile":              fileContent(t, "./ethereum/Makefile"),
				"substreams.yaml":       fileContent(t, "./ethereum/results/graphsink/substreams.yaml"),
				"rust-toolchain.toml":   fileContent(t, "./ethereum/rust-toolchain.toml"),
				"schema.sql":            fileContent(t, "./ethereum/schema.sql"),
				"schema.graphql":        fileContent(t, "./ethereum/schema.graphql"),
				"subgraph.yaml":         fileContent(t, "./ethereum/subgraph.yaml"),
			},
			choice:    codegen.SinkChoiceGraph,
			assertion: require.NoError,
		},
		{
			name: "multiple contracts - graph sink ",
			args: []args{
				{
					address:   "0x23581767a106ae21c074b2276d25e5c3e136a68b",
					abi:       fileContent(t, "ethereum/results/multiple_contracts/abi/moonbird_contract.abi.json"),
					shortName: "moonbird",
				},
				{
					address:   "0xbc4ca0eda7647a8ab7c2061c2e118a18a936f13d",
					abi:       fileContent(t, "ethereum/results/multiple_contracts/abi/bayc_contract.abi.json"),
					shortName: "bayc",
				},
			},
			want: map[string][]byte{
				"abi/bayc_contract.abi.json":     fileContent(t, "ethereum/results/multiple_contracts/abi/bayc_contract.abi.json"),
				"abi/moonbird_contract.abi.json": fileContent(t, "ethereum/results/multiple_contracts/abi/moonbird_contract.abi.json"),
				"proto/contract.proto":           fileContent(t, "./ethereum/results/multiple_contracts/proto/contract.proto"),
				"src/abi/mod.rs":                 fileContent(t, "./ethereum/results/multiple_contracts/src/abi/mod.rs"),
				"src/pb/mod.rs":                  fileContent(t, "./ethereum/results/multiple_contracts/src/pb/mod.rs"),
				"src/lib.rs":                     fileContent(t, "./ethereum/results/multiple_contracts/src/lib.rs"),
				"build.rs":                       fileContent(t, "./ethereum/results/multiple_contracts/build.rs"),
				"Cargo.lock":                     fileContent(t, "./ethereum/Cargo.lock"),
				"Cargo.toml":                     fileContent(t, "./ethereum/results/Cargo.toml"),
				"Makefile":                       fileContent(t, "./ethereum/Makefile"),
				"substreams.yaml":                fileContent(t, "./ethereum/results/graphsink/substreams.yaml"),
				"rust-toolchain.toml":            fileContent(t, "./ethereum/rust-toolchain.toml"),
				"schema.sql":                     fileContent(t, "./ethereum/results/multiple_contracts/schema.sql"),
				"schema.graphql":                 fileContent(t, "./ethereum/results/multiple_contracts/schema.graphql"),
				"subgraph.yaml":                  fileContent(t, "./ethereum/subgraph.yaml"),
			},
			choice:    codegen.SinkChoiceGraph,
			assertion: require.NoError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chain := EthereumChainsByID["Mainnet"]
			var ethereumContracts []*EthereumContract
			for _, arg := range tt.args {
				abi, err := eth.ParseABIFromBytes(arg.abi)
				require.NoError(t, err)

				ethereumContracts = append(ethereumContracts, NewEthereumContract(
					arg.shortName,
					eth.MustNewAddress("0xbc4ca0eda7647a8ab7c2061c2e118a18a936f13d"),
					nil,
					abi,
					string(arg.abi),
				))
			}

			for _, contract := range ethereumContracts {
				events, err := BuildEventModels(contract, len(ethereumContracts) > 1)
				require.NoError(t, err)
				contract.SetEvents(events)
			}

			project, err := NewEthereumProject(
				"substreams-init-test",
				"substreams_init_test",
				chain,
				ethereumContracts,
				123,
				tt.choice,
			)
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

func TestProtoFieldName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "no starting underscore",
			input:    "tokenId",
			expected: "tokenId",
		},
		{
			name:     "input starting with an underscore",
			input:    "_tokenId",
			expected: "u_tokenId",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, test.expected, sanitizeProtoFieldName(test.input))
		})
	}
}

func fileContent(t *testing.T, path string) []byte {
	content, err := os.ReadFile(path)
	require.NoError(t, err)

	return content
}
