package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/streamingfast/substreams/codegen"

	"github.com/spf13/cobra"
	"github.com/streamingfast/substreams/manifest"
)

var codegenCmd = &cobra.Command{
	Use:          "codegen <package>",
	Short:        "Generate substreams code from your substreams yaml manifest file",
	RunE:         runCodeGen,
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
}

func init() {
	rootCmd.AddCommand(codegenCmd)
}

func runCodeGen(cmd *cobra.Command, args []string) error {
	manifestPath := args[0]
	manifestReader := manifest.NewReader(manifestPath, manifest.SkipSourceCodeReader())

	pkg, err := manifestReader.Read()
	if err != nil {
		return fmt.Errorf("reading manifest %q: %w", manifestPath, err)
	}

	genFolderLocation := "./src/gen"
	if err := os.MkdirAll(genFolderLocation, os.ModePerm); err != nil {
		return fmt.Errorf("creating gen directory %v: %w", genFolderLocation, err)
	}

	generatedFilename := "generated.rs"

	fi, err := os.Create(filepath.Join(genFolderLocation, filepath.Base(generatedFilename)))
	if err != nil {
		panic(err)
	}
	defer func() {
		if err := fi.Close(); err != nil {
			panic(err)
		}
	}()

	g := codegen.NewGenerator(pkg, fi)
	err = g.Generate()

	//todo:
	// 1- create ./gen/generated.rs
	// 2- generate code in generated.rs from manifest
	// 3- add tests for generator

	//generatedFilename := "generated.rs"
	//if err := os.WriteFile(generatedFilename, , os.ModePerm); err != nil {
	//	return fmt.Errorf("writing %v file: %w", generatedFilename, err)
	//}

	return nil
}
