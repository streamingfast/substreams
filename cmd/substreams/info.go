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

	outputSinkconfigFilesPath := mustGetString(cmd, "output-sinkconfig-files-path")

	info, err := info.Extended(manifestPath, outputModule, sflags.MustGetBool(cmd, "skip-package-validation"))
	if err != nil {
		return err
	}

	if mustGetBool(cmd, "json") {
		res, err := json.MarshalIndent(info, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(res))
		return nil
	}

	fmt.Println("Package name:", info.Name)
	fmt.Println("Version:", info.Version)
	if doc := info.Documentation; doc != nil && *doc != "" {
		fmt.Println("Doc: " + strings.Replace(*doc, "\n", "\n  ", -1))
	}
	if info.Image != nil {
		fmt.Printf("Image: [embedded image: %d bytes]\n", len(info.Image))
	}

	fmt.Println("Modules:")
	fmt.Println("----")
	for _, mod := range info.Modules {
		fmt.Println("Name:", mod.Name)
		fmt.Println("Initial block:", mod.InitialBlock)
		fmt.Println("Kind:", mod.Kind)

		switch mod.Kind {
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

	if outputModule != "" {
		stages := info.ExecutionStages
		for i, layers := range stages {
			var layerDefs []string
			for _, l := range layers {
				var mods []string
				for _, m := range l {
					mods = append(mods, m)
				}
				layerDefs = append(layerDefs, fmt.Sprintf(`["%s"]`, strings.Join(mods, `","`)))
			}
			fmt.Printf("Stage %d: [%s]\n", i, strings.Join(layerDefs, `,`))
		}
	}

	if info.SinkInfo != nil {
		fmt.Println("Sink config:")
		fmt.Println("----")
		fmt.Println("type:", info.SinkInfo.TypeUrl)

		fmt.Println("configs:")
		fmt.Println(info.SinkInfo.Configs)

		if outputSinkconfigFilesPath != "" && info.SinkInfo.Files != nil {
			if err := os.MkdirAll(outputSinkconfigFilesPath, 0755); err != nil {
				return err
			}
			fmt.Println("output files:")
			for k, v := range info.SinkInfo.Files {
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
