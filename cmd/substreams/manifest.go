package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/streamingfast/substreams/manifest"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

// var manifestCmd = &cobra.Command{
// 	Use:          "manifest",
// 	SilenceUsage: true,
// }
var manifestInfoCmd = &cobra.Command{
	Use:          "info <manifest_file>",
	RunE:         runManifestInfo,
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
}

var manifestGraphCmd = &cobra.Command{
	Use:          "graph <manifest_file>",
	RunE:         runManifestGraph,
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
}

func init() {
	rootCmd.AddCommand(manifestInfoCmd)
	rootCmd.AddCommand(manifestGraphCmd)
}

func runManifestInfo(cmd *cobra.Command, args []string) error {

	fmt.Println("Manifest Info")

	manifestPath := args[0]
	manifestReader := manifest.NewReader(manifestPath)
	pkg, err := manifestReader.Read()
	if err != nil {
		return fmt.Errorf("read manifest %q: %w", manifestPath, err)
	}

	graph, err := manifest.NewModuleGraph(pkg.Modules.Modules)
	if err != nil {
		return fmt.Errorf("creating module graph: %w", err)
	}

	fmt.Println("Version:", pkg.PackageMeta[0].Version)
	if doc := pkg.PackageMeta[0].Doc; doc != "" {
		fmt.Println("Doc: " + strings.Replace(doc, "\n", "\n  ", -1))
	}

	fmt.Println("Modules:")
	fmt.Println("----")
	for _, module := range pkg.Modules.Modules {
		fmt.Println("Name:", module.Name)
		kind := module.GetKind()
		switch v := kind.(type) {
		case *pbsubstreams.Module_KindMap_:
			fmt.Println("Kind: Map")
			fmt.Println("Output Type: ", v.KindMap.OutputType)
		case *pbsubstreams.Module_KindStore_:
			fmt.Println("Kind: Store")
			fmt.Println("Value Type: ", v.KindStore.ValueType)
			fmt.Println("Update Policy: ", v.KindStore.UpdatePolicy)
		default:
			fmt.Println("Kind: Unknown")
		}
		fmt.Println("Hash:", manifest.HashModuleAsString(pkg.Modules, graph, module))
		fmt.Println("")
	}

	return nil
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
