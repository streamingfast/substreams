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

	modRs := "mod.rs"

	modRsFile, err := os.Create(filepath.Join(genFolderLocation, filepath.Base(modRs)))
	if err != nil {
		panic(err)
	}
	defer func() {
		if err := modRsFile.Close(); err != nil {
			panic(err)
		}
	}()
	g := codegen.NewGenerator(pkg, modRsFile)
	err = g.GenerateModRs()

	generatedRs := "generated.rs"

	generatedRsFile, err := os.Create(filepath.Join(genFolderLocation, filepath.Base(generatedRs)))
	if err != nil {
		panic(err)
	}
	defer func() {
		if err := generatedRsFile.Close(); err != nil {
			panic(err)
		}
	}()
	g = codegen.NewGenerator(pkg, generatedRsFile)
	err = g.Generate()

	//todo:
	// 1- create ./gen/generated.rs
	// 2- generate code in generated.rs from manifest
	// 3- add tests for generator

	//generatedRs := "generated.rs"
	//if err := os.WriteFile(generatedRs, , os.ModePerm); err != nil {
	//	return fmt.Errorf("writing %v file: %w", generatedRs, err)
	//}

	return nil
}
