package main

import (
	"errors"
	"fmt"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/streamingfast/cli"
	models "github.com/streamingfast/substreams/cmd/substreams/init-models"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/streamingfast/substreams/codegen"
)

var initCmd = &cobra.Command{
	Use:   "init [<path>]",
	Short: "Initialize a new, working Substreams project from scratch.",
	Long: cli.Dedent(`
		Initialize a new, working Substreams project from scratch. The path parameter is optional,
		with your current working directory being the default value.
	`),
	RunE:         runSubstreamsInitE,
	Args:         cobra.RangeArgs(0, 1),
	SilenceUsage: true,
}

func init() {
	alphaCmd.AddCommand(initCmd)
}

func runSubstreamsInitE(cmd *cobra.Command, args []string) error {
	srcDir, err := filepath.Abs("/tmp/test")
	if err != nil {
		return fmt.Errorf("getting absolute path of working directory: %w", err)
	}
	if len(args) == 1 {
		srcDir, err = filepath.Abs(args[0])
		if err != nil {
			return fmt.Errorf("getting absolute path of given directory: %w", err)
		}
	}

	// Bubble Tea model for questionnaire
	questionnaireModel, err := tea.NewProgram(models.NewQuestionnaire()).Run()
	if err != nil {
		return fmt.Errorf("creating questionnaire: %w", err)
	}
	questionnaireExposed := questionnaireModel.(models.Questionnaire)
	networkSelected := questionnaireExposed.Network.Selected
	nameSelected := questionnaireExposed.ProjectName.TextInput.Value()

	if networkSelected == "other" {
		fmt.Printf("We haven't added any templates for your selected chain quite yet...\n\n")
		fmt.Printf("Come join us in discord at https://discord.gg/u8amUbGBgF and suggest templates/chains you want to see!\n\n")
		return nil
	} else {
		fmt.Printf("\033[32m ✔"+"\033[0m"+" Name: %s\n", nameSelected)
		fmt.Printf("\033[32m ✔"+"\033[0m"+" Network: %s\n\n", networkSelected)
	}

	gen := codegen.NewProjectGenerator(srcDir, nameSelected)
	if _, err := os.Stat(filepath.Join(srcDir, nameSelected)); errors.Is(err, os.ErrNotExist) {
		err = gen.GenerateProject()
		if err != nil {
			return fmt.Errorf("generating code: %w", err)
		}
	} else {
		fmt.Printf("A Substreams project named %s already exists in the entered directory.\nTry changing the directory or project name and trying again.\n\n", nameSelected)
	}

	return nil
}
