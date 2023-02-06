package main

import (
	"errors"
	"fmt"
	"github.com/manifoldco/promptui"
	"github.com/spf13/cobra"
	"github.com/streamingfast/cli"
	"github.com/streamingfast/substreams/codegen"
	"os"
	"path/filepath"
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

	// Get desired project name
	projectNamePrompt := promptui.Prompt{
		Label:     "Project name: ",
		Templates: inputTemplate(),
	}
	projectName, err := projectNamePrompt.Run()
	if err != nil {
		return fmt.Errorf("running project name prompt: %w", err)
	}

	// Get desired network
	networkPrompt := promptui.Select{
		Label:     "Select network",
		Items:     []string{"Ethereum", "other"},
		Templates: choiceTemplate(),
		HideHelp:  true,
	}
	_, network, err := networkPrompt.Run()
	if err != nil {
		return fmt.Errorf("running network prompt: %w", err)
	}

	if network == "other" {
		fmt.Printf("We haven't added any templates for your selected chain quite yet...\n\n")
		fmt.Printf("Come join us in discord at https://discord.gg/u8amUbGBgF and suggest templates/chains you want to see!\n\n")
		return nil
	}

	fmt.Println("")

	gen := codegen.NewProjectGenerator(srcDir, projectName)
	if _, err := os.Stat(filepath.Join(srcDir, projectName)); errors.Is(err, os.ErrNotExist) {
		err = gen.GenerateProject()
		if err != nil {
			return fmt.Errorf("generating code: %w", err)
		}
	} else {
		fmt.Printf("A Substreams project named %s already exists in the entered directory.\nTry changing the directory or project name and trying again.\n\n", projectName)
	}

	return nil
}

func choiceTemplate() *promptui.SelectTemplates {
	return &promptui.SelectTemplates{
		Selected: fmt.Sprintf("%s {{ . | green }}", promptui.IconGood),
	}
}

func inputTemplate() *promptui.PromptTemplates {
	return &promptui.PromptTemplates{
		Prompt:  "{{ . }} ",
		Valid:   fmt.Sprintf("%s {{ . | bold }}", promptui.IconBad),
		Success: fmt.Sprintf("%s {{ . | green }}", promptui.IconGood),
	}
}
