package main

import (
	"fmt"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/streamingfast/cli"
	init_models "github.com/streamingfast/substreams/cmd/substreams/init-models"
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
	questionnaireModel, err := tea.NewProgram(init_models.NewQuestionnaire()).Run()
	if err != nil {
		return fmt.Errorf("creating questionnaire: %w", err)
	}
	questionnaireExposed := questionnaireModel.(init_models.Questionnaire)
	networkSelected := questionnaireExposed.Network.Selected
	nameSelected := questionnaireExposed.ProjectName.TextInput.Value()

	if networkSelected == "other" {
		fmt.Println("We haven't added any templates for your selected chain quite yet...")
		fmt.Println("Come join us in discord at https://discord.gg/u8amUbGBgF and suggest templates/chains you want to see!")
		return nil
	} else {
		fmt.Printf("\033[32m ✔"+"\033[0m"+" Name: %s\n", nameSelected)
		fmt.Printf("\033[32m ✔"+"\033[0m"+" Network: %s\n\n", networkSelected)
	}

	gen := codegen.NewProjectGenerator(srcDir, nameSelected)
	err = gen.GenerateProject()
	if err != nil {
		return fmt.Errorf("generating code: %w", err)
	}

	return nil
}
