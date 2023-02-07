package main

import (
	"errors"
	"fmt"
	"github.com/manifoldco/promptui"
	"github.com/spf13/cobra"
	"github.com/streamingfast/cli"
	"github.com/streamingfast/eth-go"
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

	// Get decision on abi tracking
	abiPrompt := promptui.Select{
		Label:     "Would you like to track a particular contract",
		Items:     []string{"yes", "no"},
		Templates: choiceTemplate(),
		HideHelp:  true,
	}
	_, wantsAbi, err := abiPrompt.Run()
	if err != nil {
		return fmt.Errorf("running abi prompt: %w", err)
	}

	// Default 'Bored Ape Yacht Club' contract.
	// Used in 'github.com/streamingfast/substreams-template'
	contract := "bc4ca0eda7647a8ab7c2061c2e118a18a936f13d"
	if wantsAbi == "yes" {
		contractPrompt := promptui.Prompt{
			Label:     "Contract to track: ",
			Templates: inputTemplate(),
		}
		contract, err = contractPrompt.Run()
		if err != nil {
			return fmt.Errorf("running contract prompt: %w", err)
		}
	}

	// Clean up given contract
	contractBytes, err := eth.NewHex(contract)
	if err != nil {
		return fmt.Errorf("getting contract bytes: %w", err)
	}
	contractPretty := contractBytes.Pretty()

	abi, err := codegen.GetContractAbi("0x8a90cab2b38dba80c64b7734e58ee1db38b8992e")
	if err != nil {
		return fmt.Errorf("getting contract abi: %w", err)
	}
	ethAbi, err := eth.ParseABIFromBytes(abi)
	if err != nil {
		return fmt.Errorf("parsing abi bytes: %w", err)
	}

	fmt.Println("")

	events, err := codegen.BuildEventModels(ethAbi)
	if err != nil {
		return fmt.Errorf("build ABI event models: %w", err)
	}

	gen := codegen.NewProjectGenerator(srcDir, "new", "8a90cab2b38dba80c64b7734e58ee1db38b8992e", string(abi), events)
	if _, err := os.Stat(filepath.Join(srcDir, "new")); errors.Is(err, os.ErrNotExist) {
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
