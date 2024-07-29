package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/streamingfast/cli"
	"github.com/streamingfast/cli/sflags"
	"github.com/streamingfast/substreams/info"

	"github.com/spf13/cobra"
)

func init() {
	infoCmd.Flags().String("output-sinkconfig-files-path", "", "if non-empty, any sinkconfig field of type 'bytes' that was packed from a file will be written to that path")
	infoCmd.Flags().Bool("skip-package-validation", false, "Do not perform any validation when reading substreams package")
	infoCmd.Flags().Uint64("first-streamable-block", 0, "Apply a chain's 'first-streamable-block' to modules, possibly affecting their initialBlock and hashes")
	infoCmd.Flags().Bool("used-modules-only", false, "When set, only modules that are used by the output module will be displayed (requires the output_module arg to be set)")
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
	infoCmd.Flags().Bool("json", false, "Output as JSON")
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

	outputSinkconfigFilesPath := sflags.MustGetString(cmd, "output-sinkconfig-files-path")
	firstStreamableBlock := sflags.MustGetUint64(cmd, "first-streamable-block")
	skipPackageValidation := sflags.MustGetBool(cmd, "skip-package-validation")
	onlyShowUsedModules := sflags.MustGetBool(cmd, "used-modules-only")

	if onlyShowUsedModules && outputModule == "" {
		return fmt.Errorf("used-modules-only flag requires the output_module arg to be set")
	}

	pkgInfo, err := info.Extended(manifestPath, outputModule, skipPackageValidation, firstStreamableBlock)
	if err != nil {
		return err
	}
	usedModules := make(map[string]bool)
	if outputModule != "" {
		for _, layers := range pkgInfo.ExecutionStages {
			for _, l := range layers {
				for _, mod := range l {
					usedModules[mod] = true
				}
			}
		}
	}

	if onlyShowUsedModules {
		strippedModules := make([]info.ModulesInfo, 0, len(pkgInfo.Modules))
		for _, mod := range pkgInfo.Modules {
			if usedModules[mod.Name] {
				strippedModules = append(strippedModules, mod)
			}
		}
		pkgInfo.Modules = strippedModules
	}

	if sflags.MustGetBool(cmd, "json") {
		res, err := json.MarshalIndent(pkgInfo, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(res))
		return nil
	}

	fmt.Println("Package name:", pkgInfo.Name)
	fmt.Println("Version:", pkgInfo.Version)
	if doc := pkgInfo.Documentation; doc != nil && *doc != "" {
		fmt.Println("Doc: " + strings.Replace(*doc, "\n", "\n  ", -1))
	}
	if pkgInfo.Image != nil {
		fmt.Printf("Image: [embedded image: %d bytes]\n", len(pkgInfo.Image))
	}

	fmt.Println("Modules:")
	fmt.Println("----")
	for _, mod := range pkgInfo.Modules {
		fmt.Println("Name:", mod.Name)
		fmt.Println("Initial block:", mod.InitialBlock)
		fmt.Println("Kind:", mod.Kind)
		for _, input := range mod.Inputs {
			fmt.Printf("Input: %s: %s\n", input.Type, input.Name)
		}
		if mod.BlockFilter != nil {
			fmt.Printf("Block Filter: (using *%s*): `%s`\n", mod.BlockFilter.Module, mod.BlockFilter.Query)
		}

		switch mod.Kind {
		case "index":
			fmt.Println("Output Type:", *mod.OutputType)
		case "map":
			fmt.Println("Output Type:", *mod.OutputType)
		case "store":
			fmt.Println("Value Type:", *mod.ValueType)
			fmt.Println("Update Policy:", *mod.UpdatePolicy)
		default:
			fmt.Println("Kind: Unknown")
		}

		fmt.Println("Hash:", mod.Hash)
		if doc := mod.Documentation; doc != nil && *doc != "" {
			fmt.Println("Doc: ", *doc)
		}
		fmt.Println("")
	}

	if pkgInfo.Network != "" {
		fmt.Printf("Network: %s\n", pkgInfo.Network)
		fmt.Println("")
	}

	if pkgInfo.Networks != nil {
		fmt.Println("Networks:")
		for network, params := range pkgInfo.Networks {
			fmt.Printf("  %s:\n", network)
			if params.InitialBlocks != nil {
				fmt.Println("    Initial Blocks:")
			}
			for mod, start := range params.InitialBlocks {
				fmt.Printf("      - %s: %d\n", mod, start)
			}
			if params.Params != nil {
				fmt.Println("    Params:")
			}
			for mod, p := range params.Params {
				fmt.Printf("      - %s: %q\n", mod, p)
			}
			fmt.Println("")
		}
	}

	if outputModule != "" {
		stages := pkgInfo.ExecutionStages
		for i, layers := range stages {
			var layerDefs []string
			for _, l := range layers {
				var mods []string
				mods = append(mods, l...)
				layerDefs = append(layerDefs, fmt.Sprintf(`["%s"]`, strings.Join(mods, `","`)))
			}
			fmt.Printf("Stage %d: [%s]\n", i, strings.Join(layerDefs, `,`))
		}
	}

	if pkgInfo.SinkInfo != nil {
		fmt.Println("Sink config:")
		fmt.Println("----")
		fmt.Println("type:", pkgInfo.SinkInfo.TypeUrl)

		fmt.Println("configs:")
		fmt.Println(pkgInfo.SinkInfo.Configs)

		if outputSinkconfigFilesPath != "" && pkgInfo.SinkInfo.Files != nil {
			if err := os.MkdirAll(outputSinkconfigFilesPath, 0755); err != nil {
				return err
			}
			fmt.Println("output files:")
			for k, v := range pkgInfo.SinkInfo.Files {
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
