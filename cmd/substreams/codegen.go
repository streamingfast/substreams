package main

import (
	"fmt"
	"path"
	"path/filepath"

	"github.com/streamingfast/cli"
	"github.com/streamingfast/substreams/tools"

	"github.com/jhump/protoreflect/desc"
	"github.com/spf13/cobra"
	"github.com/streamingfast/substreams/codegen"
	"github.com/streamingfast/substreams/manifest"
)

var codegenCmd = &cobra.Command{
	Use:   "codegen [<manifest>]",
	Short: "Generate a Rust trait and boilerplate code from your 'substreams.yaml' for nicer development",
	Long: cli.Dedent(`
		Generate a Rust trait and boilerplate code from your 'substreams.yaml' for nicer development. 
		The manifest is optional as it will try to find a file named 'substreams.yaml' in current working directory if nothing entered.
		You may enter a directory that contains a 'substreams.yaml' file in place of '<manifest_file>'.
	`),
	RunE: runCodeGen,
	Args: cobra.RangeArgs(0, 1),
}

func init() {
	alphaCmd.AddCommand(codegenCmd)
}

func runCodeGen(cmd *cobra.Command, args []string) error {
	manifestPathRaw := ""
	if len(args) == 1 {
		manifestPathRaw = args[0]
	}
	manifestPath, err := tools.ResolveManifestFile(manifestPathRaw)
	if err != nil {
		return fmt.Errorf("resolving manifest: %w", err)
	}

	var protoDefinitions []*desc.FileDescriptor
	manifestReader := manifest.NewReader(manifestPath, manifest.SkipSourceCodeReader(), manifest.WithCollectProtoDefinitions(func(pd []*desc.FileDescriptor) {
		protoDefinitions = pd
	}))

	manifestAbsPath, err := filepath.Abs(manifestPath)
	if err != nil {
		return fmt.Errorf("computing working directory: %w", err)
	}
	workingDir := filepath.Dir(manifestAbsPath)
	manif, err := manifest.LoadManifestFile(manifestAbsPath)
	if err != nil {
		return fmt.Errorf("loading manifest: %w", err)
	}
	pkg, err := manifestReader.Read()
	if err != nil {
		return fmt.Errorf("reading manifest %q: %w", manifestPath, err)
	}

	srcDir := path.Join(workingDir, "src")

	gen := codegen.NewGenerator(pkg, manif, protoDefinitions, srcDir)
	err = gen.Generate()
	if err != nil {
		return fmt.Errorf("generating code: %w", err)
	}

	return nil
}
