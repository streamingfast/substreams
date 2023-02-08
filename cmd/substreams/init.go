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
	"regexp"
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

	projectName, err := generatePrompt("Project name: ")
	if err != nil {
		return fmt.Errorf("running project name prompt: %w", err)
	}

	network, err := generateSelect("Select network", []string{"Ethereum", "Other"})
	if err != nil {
		return fmt.Errorf("running network prompt: %w", err)
	}

	if network == "other" {
		fmt.Printf("We haven't added any templates for your selected chain quite yet...\n\n")
		fmt.Printf("Come join us in discord at https://discord.gg/u8amUbGBgF and suggest templates/chains you want to see!\n\n")
		return nil
	}

	wantsABI, err := generateSelect("Would you like to track a particular contract", []string{"yes", "no"})
	if err != nil {
		return fmt.Errorf("running ABI prompt: %w", err)
	}

	// Default 'Bored Ape Yacht Club' contract.
	// Used in 'github.com/streamingfast/substreams-template'
	contract, _ := eth.NewAddress("bc4ca0eda7647a8ab7c2061c2e118a18a936f13d")
	if wantsABI == "yes" {
		contract, err = generateContractPrompt("Verified Ethereum mainnet contract to track: ")
		if err != nil {
			return fmt.Errorf("running contract prompt: %w", err)
		}
	}
	// Get contract ABI & parse
	contractPretty := contract.Pretty()
	ABI, ethABI, err := codegen.GetContractABI(contractPretty)
	if err != nil {
		return fmt.Errorf("getting contract ABI: %w", err)
	}

	fmt.Println("")

	events, err := codegen.BuildEventModels(ethABI)
	if err != nil {
		return fmt.Errorf("build ABI event models: %w", err)
	}

	gen := codegen.NewProjectGenerator(srcDir, projectName, contract, string(ABI), events)
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

func generatePrompt(label string) (string, error) {
	moduleNameRegexp := regexp.MustCompile(`^([a-zA-Z][a-zA-Z0-9_]{0,63})$`)
	namingValidation := func(input string) error {
		ok := moduleNameRegexp.MatchString(input)
		if !ok {
			return errors.New("invalid name: must match ^([a-zA-Z][a-zA-Z0-9_]{0,63})$")
		}
		return nil
	}
	prompt := promptui.Prompt{
		Label:     label,
		Templates: inputTemplate(),
		Validate:  namingValidation,
	}
	choice, err := prompt.Run()
	if err != nil {
		return "", fmt.Errorf("running prompt: %w", err)
	}

	return choice, nil
}

func generateContractPrompt(label string) (eth.Address, error) {
	contractValidation := func(input string) error {
		_, err := eth.NewAddress(input)
		if err != nil {
			return errors.New("Invalid address")
		}
		return nil
	}
	prompt := promptui.Prompt{
		Label:     label,
		Templates: inputTemplate(),
		Validate:  contractValidation,
	}

	choice, err := prompt.Run()
	if err != nil {
		return nil, fmt.Errorf("running prompt: %w", err)
	}

	// Clean up given contract
	contractAddress, err := eth.NewAddress(choice)
	if err != nil {
		return nil, fmt.Errorf("getting contract bytes: %w", err)
	}

	return contractAddress, nil
}

func generateSelect(label string, items []string) (string, error) {
	choice := promptui.Select{
		Label:     label,
		Items:     items,
		Templates: choiceTemplate(),
		HideHelp:  true,
	}

	_, selection, err := choice.Run()
	if err != nil {
		return "", fmt.Errorf("running network prompt: %w", err)
	}

	return selection, nil
}
