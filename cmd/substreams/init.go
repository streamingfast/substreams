package main

import (
	"fmt"
	"github.com/streamingfast/cli"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/streamingfast/substreams/codegen"
)

var initCmd = &cobra.Command{
	Use:   "init <projectName> [<path>]",
	Short: "Initialize a new, working Substreams project from scratch.",
	Long: cli.Dedent(`
		Initialize a new, working Substreams project from scratch. The path parameter is optional, 
		with your current working directory being the default value.
	`),
	RunE:         runSubstreamsInitE,
	Args:         cobra.RangeArgs(1, 2),
	SilenceUsage: true,
}

func init() {
	alphaCmd.AddCommand(initCmd)
}

func runSubstreamsInitE(cmd *cobra.Command, args []string) error {
	projectName := args[0]

	path, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}
	if len(args) == 2 {
		path = args[1]
	}

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
