package main

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/streamingfast/substreams/manifest"
)

var graphCmd = &cobra.Command{
	Use:          "graph <manifest_file>",
	Short:        "GenerateProto mermaid-js graph document",
	RunE:         runManifestGraph,
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
}

func init() {
	rootCmd.AddCommand(graphCmd)
}

func runManifestGraph(cmd *cobra.Command, args []string) error {
	manifestPath := args[0]
	manifestReader := manifest.NewReader(manifestPath)
	pkg, err := manifestReader.Read()
	if err != nil {
		return fmt.Errorf("read manifest %q: %w", manifestPath, err)
	}

	manifest.PrintMermaid(pkg.Modules)

	return nil
}
