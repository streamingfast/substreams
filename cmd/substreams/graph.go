package main

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/streamingfast/cli"

	"github.com/streamingfast/substreams/manifest"
)

var graphCmd = &cobra.Command{
	Use:   "graph [<manifest_file>]",
	Short: "Generate mermaid-js graph document",
	RunE:  runManifestGraph,
	Long: cli.Dedent(`
		Generate mermaid-js graph document. The manifest is optional as it will try to find a file named
		'substreams.yaml' in current working directory if nothing entered. You may enter a directory that contains a
		'substreams.yaml' file in place of '<manifest_file>', or a link to a remote .spkg file, using urls gs://, http(s)://, ipfs://, etc.'.
	`),
	Args:         cobra.RangeArgs(0, 1),
	SilenceUsage: true,
}

func init() {
	rootCmd.AddCommand(graphCmd)
}

func runManifestGraph(cmd *cobra.Command, args []string) error {
	manifestPath := ""
	if len(args) == 1 {
		manifestPath = args[0]
	}

	manifestReader, err := manifest.NewReader(manifestPath, getReaderOpts(cmd)...)
	if err != nil {
		return fmt.Errorf("manifest reader: %w", err)
	}

	pkg, err := manifestReader.Read()
	if err != nil {
		return fmt.Errorf("read manifest %q: %w", manifestPath, err)
	}

	manifest.PrintMermaid(pkg.Modules)

	fmt.Println("")
	fmt.Println("Here is a quick link to see the graph:")
	fmt.Println("")
	fmt.Println(manifest.GenerateMermaidLiveURL(pkg.Modules))

	return nil
}
