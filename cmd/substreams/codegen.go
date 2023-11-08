package main

import (
	"fmt"
	"os"
	"path"
	"path/filepath"

	"github.com/streamingfast/cli"

	"github.com/jhump/protoreflect/desc"
	"github.com/spf13/cobra"
	"github.com/streamingfast/substreams/codegen"
	"github.com/streamingfast/substreams/manifest"
)

var (
	devSubstreamsCodegenGenerateTo = os.Getenv("SUBSTREAMS_DEV_CODEGEN_GENERATE_TO")
)

var codegenCmd = &cobra.Command{
	Use:   "codegen [<manifest>]",
	Short: "Generate a Rust trait and boilerplate code from your 'substreams.yaml' for nicer development",
	Long: cli.Dedent(`
		Generate a Rust trait and boilerplate code from your 'substreams.yaml' for nicer development.
		The manifest is optional as it will try to find a file named 'substreams.yaml' in current working directory if nothing entered.
		You may enter a directory that contains a 'substreams.yaml' file in place of '<manifest_file>', or a link to a remote .spkg file, 
		using urls gs://, http(s)://, ipfs://, etc.'.
	`),
	RunE: runCodeGen,
	Args: cobra.RangeArgs(0, 1),
}

func init() {
	alphaCmd.AddCommand(codegenCmd)
}

func runCodeGen(cmd *cobra.Command, args []string) error {
	manifestPath := ""
	if len(args) == 1 {
		manifestPath = args[0]
	}

	var protoDefinitions []*desc.FileDescriptor

	readerOpts := append(getReaderOpts(cmd), manifest.SkipSourceCodeReader(), manifest.WithCollectProtoDefinitions(func(pd []*desc.FileDescriptor) {
		protoDefinitions = pd
	}))

	manifestReader, err := manifest.NewReader(manifestPath, readerOpts...)
	if err != nil {
		return fmt.Errorf("manifest reader: %w", err)
	}

	manifestAbsPath, err := filepath.Abs(manifestPath)
	if err != nil {
		return fmt.Errorf("computing working directory: %w", err)
	}
	workingDir := filepath.Dir(manifestAbsPath)
	manif, err := manifest.LoadManifestFile(manifestAbsPath, workingDir)
	if err != nil {
		return fmt.Errorf("loading manifest: %w", err)
	}
	pkg, err := manifestReader.Read()
	if err != nil {
		return fmt.Errorf("reading manifest %q: %w", manifestPath, err)
	}

	srcDir := path.Join(workingDir, "src")
	if devSubstreamsCodegenGenerateTo != "" {
		srcDir, err = filepath.Abs(devSubstreamsCodegenGenerateTo)
		if err != nil {
			panic(fmt.Errorf("generate to folder %q should be able to be made absolute: %w", devSubstreamsCodegenGenerateTo, err))
		}
	}

	gen := codegen.NewGenerator(pkg, manif, protoDefinitions, srcDir)
	err = gen.Generate()
	if err != nil {
		return fmt.Errorf("generating code: %w", err)
	}

	return nil
}
