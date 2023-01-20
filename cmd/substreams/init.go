package main

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/streamingfast/substreams/codegen"
)

var initCmd = &cobra.Command{
	Use:          "init <projectName> <path>",
	Short:        "Initialize a new Substreams project",
	RunE:         runSubstreamsInitE,
	Args:         cobra.ExactArgs(2),
	SilenceUsage: true,
}

func init() {
	alphaCmd.AddCommand(initCmd)
}

func runSubstreamsInitE(cmd *cobra.Command, args []string) error {
	projectName := args[0]
	path := args[1]

	srcDir, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("getting absolute path of %q: %w", path, err)
	}

	gen := codegen.NewProjectGenerator(srcDir, projectName)
	err = gen.GenerateProject()
	if err != nil {
		return fmt.Errorf("generating code: %w", err)
	}

	return nil
}
