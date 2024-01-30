package main

import (
	"fmt"
	"os"

	"github.com/streamingfast/cli"
	"github.com/streamingfast/eth-go"
	"github.com/streamingfast/substreams/codegen/templates"
)

//go:generate go run .

func main() {
	abiContent, err := os.ReadFile("./abi/contract.abi.json")
	cli.NoError(err, "Unable to read ABI file content")

	abi, err := eth.ParseABIFromBytes(abiContent)
	cli.NoError(err, "Unable to parse ABI file content")

	chain := templates.EthereumChainsByID["Mainnet"]

	ethereumContracts := []*templates.EthereumContract{templates.NewEthereumContract(
		"substreams-init-tests",
		eth.MustNewAddress("0xbc4ca0eda7647a8ab7c2061c2e118a18a936f13d"),
		abi,
		string(abiContent),
	)}

	project, err := templates.NewEthereumProject(
		"substreams-init-test",
		"substreams_init_test",
		chain,
		ethereumContracts,
		123,
	)

	cli.NoError(err, "Unable to create Ethereum project")

	files, err := project.Render()
	cli.NoError(err, "Unable to render Ethereum project")

	for _, fileToWrite := range []string{"proto/contract.proto", "src/lib.rs", "Cargo.toml", "substreams.yaml", "Makefile"} {
		content, found := files[fileToWrite]
		cli.Ensure(found, "The file %q is not rendered by Ethereum project", fileToWrite)

		err = os.WriteFile(fileToWrite, content, os.ModePerm)
		cli.NoError(err, "Unable to write Ethereum rendered file %q: %w", fileToWrite, err)

		fmt.Printf("Ethereum project template file %q rendered\n", fileToWrite)
	}
}
