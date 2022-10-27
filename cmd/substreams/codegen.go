package main

import (
	"fmt"
	"path"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/streamingfast/substreams/codegen"
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

	manifestAbsPath, err := filepath.Abs(manifestPath)
	if err != nil {
		return fmt.Errorf("computing working directory: %w", err)
	}
	workingDir := filepath.Dir(manifestAbsPath)

	pkg, err := manifestReader.Read()
	if err != nil {
		return fmt.Errorf("reading manifest %q: %w", manifestPath, err)
	}

	srcDir := path.Join(workingDir, "src")

	gen := codegen.NewGenerator(pkg, srcDir)
	err = gen.Generate()
	if err != nil {
		return fmt.Errorf("generating code: %w", err)
	}

	//modRs := "mod.rs"
	//
	//modRsFile, err := os.Create(filepath.Join(genFolderLocation, filepath.Base(modRs)))
	//if err != nil {
	//	panic(err)
	//}
	//defer func() {
	//	if err := modRsFile.Close(); err != nil {
	//		panic(err)
	//	}
	//}()
	//g := codegen.NewGenerator(pkg, modRsFile)
	//err = g.GenerateModRs()
	//
	//generatedRs := "generated.rs"
	//
	//generatedRsFile, err := os.Create(filepath.Join(genFolderLocation, filepath.Base(generatedRs)))
	//if err != nil {
	//	panic(err)
	//}
	//defer func() {
	//	if err := generatedRsFile.Close(); err != nil {
	//		panic(err)
	//	}
	//}()
	//g = codegen.NewGenerator(pkg, generatedRsFile)
	//err = g.Generate()
	//
	////todo:
	//// 1- create ./gen/generated.rs
	//// 2- generate code in generated.rs from manifest
	//// 3- add tests for generator
	//
	////generatedRs := "generated.rs"
	////if err := os.WriteFile(generatedRs, , os.ModePerm); err != nil {
	////	return fmt.Errorf("writing %v file: %w", generatedRs, err)
	////}

	return nil
}
