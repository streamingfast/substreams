package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/streamingfast/cli"

	"github.com/spf13/cobra"
	"github.com/streamingfast/substreams/manifest"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/pipeline/outputmodules"
)

func init() {
	infoCmd.Flags().String("output-sinkconfig-files-path", "", "if non-empty, any sinkconfig field of type 'bytes' that was packed from a file will be written to that path")
}

var infoCmd = &cobra.Command{
	Use:   "info [<manifest_file> [<output_module>]]",
	Short: "Display package modules and docs",
	Long: cli.Dedent(`
		Display package modules and docs. The manifest is optional as it will try to find a file named
		'substreams.yaml' in current working directory if nothing entered. You may enter a directory that contains
		a 'substreams.yaml' file in place of '<manifest_file>, or a link to a remote .spkg file, using urls gs://, http(s)://, ipfs://, etc.'.
		Specify an "output_module" to see how processing can be divided in different stages to produce the requested output.
	`),
	RunE:         runInfo,
	Args:         cobra.RangeArgs(0, 2),
	SilenceUsage: true,
}

func init() {
	rootCmd.AddCommand(infoCmd)
}

func runInfo(cmd *cobra.Command, args []string) error {
	manifestPath := ""
	if len(args) != 0 {
		manifestPath = args[0]
	}

	var outputModule string
	if len(args) == 2 {
		outputModule = args[1]
	}

	outputSinkconfigFilesPath := mustGetString(cmd, "output-sinkconfig-files-path")

	manifestReader, err := manifest.NewReader(manifestPath)
	if err != nil {
		return fmt.Errorf("manifest reader: %w", err)
	}

	pkg, err := manifestReader.Read()
	if err != nil {
		return fmt.Errorf("read manifest %q: %w", manifestPath, err)
	}

	graph, err := manifest.NewModuleGraph(pkg.Modules.Modules)
	if err != nil {
		return fmt.Errorf("creating module graph: %w", err)
	}

	fmt.Println("Package name:", pkg.PackageMeta[0].Name)
	fmt.Println("Version:", pkg.PackageMeta[0].Version)
	if doc := pkg.PackageMeta[0].Doc; doc != "" {
		fmt.Println("Doc: " + strings.Replace(doc, "\n", "\n  ", -1))
	}

	hashes := manifest.NewModuleHashes()

	fmt.Println("Modules:")
	fmt.Println("----")
	for modIdx, module := range pkg.Modules.Modules {
		fmt.Println("Name:", module.Name)
		fmt.Println("Initial block:", module.InitialBlock)
		kind := module.GetKind()
		switch v := kind.(type) {
		case *pbsubstreams.Module_KindMap_:
			fmt.Println("Kind: map")
			fmt.Println("Output Type:", v.KindMap.OutputType)
		case *pbsubstreams.Module_KindStore_:
			fmt.Println("Kind: store")
			fmt.Println("Value Type:", v.KindStore.ValueType)
			fmt.Println("Update Policy:", v.KindStore.UpdatePolicy)
		default:
			fmt.Println("Kind: Unknown")
		}

		hashes.HashModule(pkg.Modules, module, graph)

		fmt.Println("Hash:", hashes.Get(module.Name))
		moduleMeta := pkg.ModuleMeta[modIdx]
		if moduleMeta != nil && moduleMeta.Doc != "" {
			fmt.Println("Doc: " + strings.Replace(moduleMeta.Doc, "\n", "\n  ", -1))
		}
		fmt.Println("")
	}

	if outputModule != "" {
		outputGraph, err := outputmodules.NewOutputModuleGraph(outputModule, true, pkg.Modules)
		if err != nil {
			return err
		}
		for i, layers := range outputGraph.StagedUsedModules() {
			var layerDefs []string
			for _, l := range layers {
				var mods []string
				for _, m := range l {
					mods = append(mods, m.Name)
				}
				layerDefs = append(layerDefs, fmt.Sprintf(`["%s"]`, strings.Join(mods, `","`)))
			}
			fmt.Printf("Stage %d: [%s]\n", i, strings.Join(layerDefs, `,`))
		}

	}

	if pkg.SinkConfig != nil {
		fmt.Println()
		fmt.Println("Sink config:")
		fmt.Println("----")
		fmt.Println("type:", pkg.SinkConfig.TypeUrl)

		confs, files, err := manifest.DescribeSinkConfigs(pkg)
		if err != nil {
			return err
		}

		fmt.Println("configs:")
		fmt.Println(confs)

		if outputSinkconfigFilesPath != "" && files != nil {
			if err := os.MkdirAll(outputSinkconfigFilesPath, 0755); err != nil {
				return err
			}
			fmt.Println("output files:")
			for k, v := range files {
				filename := filepath.Join(outputSinkconfigFilesPath, k)
				f, err := os.OpenFile(filename, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0644)
				if err != nil {
					return err
				}
				if _, err := f.Write(v); err != nil {
					return err
				}
				fmt.Printf("  - %q written to %q\n", k, filename)
			}
		}

	}

	return nil
}
