package main

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/streamingfast/cli"
	"github.com/streamingfast/substreams/codegen"
	"github.com/streamingfast/substreams/manifest"
	"go.uber.org/zap"
)

var protogenCmd = &cobra.Command{
	Use:   "protogen [<manifest>]",
	Short: "Generate Rust bindings from a package",
	Long: cli.Dedent(`
		Generate Rust bindings from a package. The manifest is optional as it will try to find a file named
		'substreams.yaml' in current working directory if nothing entered. You may enter a directory that contains a 'substreams.yaml'
		file in place of '<manifest_file>', or a link to a remote .spkg file, using urls gs://, http(s)://, ipfs://, etc.'.
	`),
	RunE:         runProtogen,
	Args:         cobra.RangeArgs(0, 1),
	SilenceUsage: true,
}

func init() {
	rootCmd.AddCommand(protogenCmd)

	flags := protogenCmd.Flags()
	flags.StringP("output-path", "o", "src/pb", cli.FlagDescription(`
		Directory to output generated .rs files, if the received <package> argument is a local Substreams manifest file
		(e.g. a local file ending with .yaml), the output path will be made relative to it
	`))
	flags.StringArrayP("exclude-paths", "x", []string{}, "Exclude specific files or directories, for example \"proto/a/a.proto\" or \"proto/a\"")
	flags.Bool("generate-mod-rs", true, cli.FlagDescription(`
		Generate the protobuf 'mod.rs' file alongside the rust bindings. Include '--generate-mod-rs=false' If you wish to disable this generation.
		If there is a present 'buf.gen.yaml', consult https://github.com/neoeinstein/protoc-gen-prost/blob/main/protoc-gen-prost-crate/README.md to add 'mod.rs' generation functionality.
	`))
	flags.Bool("show-generated-buf-gen", false, "Whether to show the generated buf.gen.yaml file or not")
}

func runProtogen(cmd *cobra.Command, args []string) error {
	outputPath := mustGetString(cmd, "output-path")
	excludePaths := mustGetStringArray(cmd, "exclude-paths")
	generateMod := mustGetBool(cmd, "generate-mod-rs")
	showGeneratedBufGen := mustGetBool(cmd, "show-generated-buf-gen")

	manifestPath := ""
	if len(args) == 1 {
		manifestPath = args[0]
	}

	manifestReader, err := manifest.NewReader(manifestPath, manifest.SkipSourceCodeReader(), manifest.SkipModuleOutputTypeValidationReader())
	if err != nil {
		return fmt.Errorf("manifest reader: %w", err)
	}

	if manifestReader.IsLocalManifest() && !filepath.IsAbs(outputPath) {
		newOutputPath := filepath.Join(filepath.Dir(manifestPath), outputPath)

		zlog.Debug("manifest path is a local manifest, making output path relative to it", zap.String("old", outputPath), zap.String("new", newOutputPath))
		outputPath = newOutputPath
	}

	pkg, err := manifestReader.Read()
	if err != nil {
		return fmt.Errorf("reading manifest %q: %w", manifestPath, err)
	}

	if _, err = manifest.NewModuleGraph(pkg.Modules.Modules); err != nil {
		return fmt.Errorf("processing module graph %w", err)
	}

	generator := codegen.NewProtoGenerator(outputPath, excludePaths, generateMod)
	return generator.GenerateProto(pkg, showGeneratedBufGen)
}
