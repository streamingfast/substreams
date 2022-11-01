package main

import (
	"fmt"
	"path"
	"path/filepath"

	"github.com/jhump/protoreflect/desc"

	"github.com/spf13/cobra"
	"github.com/streamingfast/substreams/codegen"
	"github.com/streamingfast/substreams/manifest"
)

var codegenCmd = &cobra.Command{
	Use:          "codegen <package>",
	Short:        "GenerateProto substreams code from your substreams yaml manifest file",
	RunE:         runCodeGen,
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
}

func init() {
	rootCmd.AddCommand(codegenCmd)
}

func runCodeGen(cmd *cobra.Command, args []string) error {
	manifestPath := args[0]

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
