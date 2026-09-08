package main

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/streamingfast/cli"
	"github.com/streamingfast/cli/sflags"
	"github.com/streamingfast/substreams/info"
	"github.com/streamingfast/substreams/manifest"

	"github.com/spf13/cobra"
)

func init() {
	infoCmd.Flags().String("output-sinkconfig-files-path", "", "if non-empty, any sinkconfig field of type 'bytes' that was packed from a file will be written to that path")
	infoCmd.Flags().Bool("skip-package-validation", false, "Do not perform any validation when reading substreams package")
	infoCmd.Flags().Bool("used-modules-only", false, "When set, only modules that are used by the output module will be displayed (requires the output_module arg to be set)")
	infoCmd.Flags().Bool("summarize-hash-types", false, "When set, will also print the hash of the modules, grouped by type and their dependencies on stores")
	infoCmd.Flags().Bool("expand-networks", false, "When set, lists the initial block and params of every module under every network instead of a summary")
	infoCmd.Flags().String("network", "", "Specify the network to use for params and initialBlocks, overriding the 'network' field in the substreams package")

	infoCmd.Flags().StringArrayP("params", "p", nil, "Set a params for parameterizable modules. Can be specified multiple times. Ex: -p module1=valA -p module2=valX&valY")

}

// renderNetworks renders the 'Networks' section of 'substreams info'. Because a packaged Substreams
// carries the initial block and params of every module under every network, the expanded form grows
// with the product of the two, so it is summarized unless explicitly asked for.
func renderNetworks(defaultNetwork string, networks map[string]*manifest.NetworkParams, expand bool) string {
	out := &strings.Builder{}
	out.WriteString("Networks:\n")

	for _, network := range slices.Sorted(maps.Keys(networks)) {
		params := networks[network]

		label := network
		if network == defaultNetwork {
			label += " (default)"
		}

		if !expand {
			fmt.Fprintf(out, "  %s: %s, %s\n", label,
				pluralize(len(params.InitialBlocks), "initial block"),
				pluralize(len(params.Params), "param"),
			)
			continue
		}

		fmt.Fprintf(out, "  %s:\n", label)
		if len(params.InitialBlocks) > 0 {
			out.WriteString("    Initial Blocks:\n")
			for _, mod := range slices.Sorted(maps.Keys(params.InitialBlocks)) {
				fmt.Fprintf(out, "      - %s: %d\n", mod, params.InitialBlocks[mod])
			}
		}
		if len(params.Params) > 0 {
			out.WriteString("    Params:\n")
			for _, mod := range slices.Sorted(maps.Keys(params.Params)) {
				fmt.Fprintf(out, "      - %s: %q\n", mod, params.Params[mod])
			}
		}
		out.WriteString("\n")
	}

	if !expand {
		out.WriteString("\n  Use --expand-networks to see the per-module values.\n")
	}

	return out.String()
}

func pluralize(count int, singular string) string {
	if count == 1 {
		return fmt.Sprintf("%d %s", count, singular)
	}

	return fmt.Sprintf("%d %ss", count, singular)
}

var infoCmd = &cobra.Command{
	Use:   "info [<manifest_file> [<output_module>]]",
	Short: "Display package modules and docs",
	Long: cli.Dedent(`
		Display package modules and docs. The manifest is optional as it will try to find a file named
		'substreams.yaml' in current working directory if nothing entered. You may enter a directory that contains
		a 'substreams.yaml' file in place of '<manifest_file>, or a link to a remote .spkg file, using urls gs://, http(s)://, ipfs://, etc.'.
		You can also use "-" to read the manifest from standard input.
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
	skipPackageValidation := sflags.MustGetBool(cmd, "skip-package-validation")
	onlyShowUsedModules := sflags.MustGetBool(cmd, "used-modules-only")
	summarizeHashTypes := sflags.MustGetBool(cmd, "summarize-hash-types")
	expandNetworks := sflags.MustGetBool(cmd, "expand-networks")
	overrideNetwork := sflags.MustGetString(cmd, "network")

	if onlyShowUsedModules && outputModule == "" {
		return fmt.Errorf("used-modules-only flag requires the output_module arg to be set")
	}

	opts := []manifest.Option{
		manifest.WithRegistryURL(getSubstreamsDownloadEndpoint()),
	}
	if skipPackageValidation {
		opts = append(opts, manifest.SkipPackageValidationReader())
	}
	if overrideNetwork != "" {
		opts = append(opts, manifest.WithOverrideNetwork(overrideNetwork))
	}

	requestParams := sflags.MustGetStringArray(cmd, "params")
	paramsStringMap := make(map[string]struct{})
	for _, parameter := range requestParams {
		moduleName := strings.Split(parameter, "=")[0]
		paramsStringMap[moduleName] = struct{}{}
	}

	if len(requestParams) != 0 {
		params, err := manifest.ParseParams(requestParams)
		if err != nil {
			return fmt.Errorf("parsing params: %w", err)
		}
		opts = append(opts, manifest.WithParams(params))
	}

	pkgInfo, err := info.Extended(manifestPath, outputModule, opts...)
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

	modMap := make(map[string]info.ModulesInfo)
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
		modMap[mod.Name] = mod
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
		fmt.Print(renderNetworks(pkgInfo.Network, pkgInfo.Networks, expandNetworks))
	}

	var mappersDependingOnStores []string
	var mappersNotDependingOnStores []string
	var indexesDependingOnStores []string
	var indexesNotDependingOnStores []string
	var stores []string

	if outputModule != "" {
		stages := pkgInfo.ExecutionStages
		for i, layers := range stages {
			var layerDefs []string
			for _, l := range layers {
				for _, mod := range l {
					switch modMap[mod].Kind {
					case "index":
						if i == 0 {
							indexesNotDependingOnStores = append(indexesNotDependingOnStores, modMap[mod].Hash)
						} else {
							indexesDependingOnStores = append(indexesDependingOnStores, modMap[mod].Hash)
						}
					case "map":
						if i == 0 {
							mappersNotDependingOnStores = append(mappersNotDependingOnStores, modMap[mod].Hash)
						} else {
							mappersDependingOnStores = append(mappersDependingOnStores, modMap[mod].Hash)
						}
					case "store":
						stores = append(stores, modMap[mod].Hash)
					}
				}
				var mods []string
				mods = append(mods, l...)
				layerDefs = append(layerDefs, fmt.Sprintf(`["%s"]`, strings.Join(mods, `","`)))
			}
			fmt.Printf("Stage %d: [%s]\n", i, strings.Join(layerDefs, `,`))
		}

		if summarizeHashTypes {
			fmt.Println("")
			if mappersDependingOnStores != nil {
				fmt.Println("Mappers depending on stores:", strings.Join(mappersDependingOnStores, " "))
			}
			if mappersNotDependingOnStores != nil {
				fmt.Println("Mappers NOT depending on stores:", strings.Join(mappersNotDependingOnStores, " "))
			}
			if indexesDependingOnStores != nil {
				fmt.Println("Indexes depending on stores:", strings.Join(indexesDependingOnStores, " "))
			}
			if indexesNotDependingOnStores != nil {
				fmt.Println("Indexes NOT depending on stores:", strings.Join(indexesNotDependingOnStores, " "))
			}
			if stores != nil {
				fmt.Println("Stores:", strings.Join(stores, " "))
			}
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
