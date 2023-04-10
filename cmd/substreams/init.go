package main

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/manifoldco/promptui"
	"github.com/spf13/cobra"
	"github.com/streamingfast/cli"
	"github.com/streamingfast/eth-go"
	"github.com/streamingfast/substreams/codegen"
	"github.com/streamingfast/substreams/codegen/templates"
)

// Some developers centric environment overidde to make it faster to iterate on `substreams init` command
var (
	devInitSourceDirectory         = os.Getenv("SUBSTREAMS_DEV_INIT_SOURCE_DIRECTORY")
	devInitProjectName             = os.Getenv("SUBSTREAMS_DEV_INIT_PROJECT_NAME")
	devInitProtocol                = os.Getenv("SUBSTREAMS_DEV_INIT_PROTOCOL")
	devInitEthereumTrackedContract = os.Getenv("SUBSTREAMS_DEV_INIT_ETHEREUM_TRACKED_CONTRACT")
)

var errInitUnsupportedChain = errors.New("unsupported chain")

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
	relativeSrcDir := "."
	if len(args) == 1 {
		relativeSrcDir = args[0]
	}

	if devInitSourceDirectory != "" {
		relativeSrcDir = devInitSourceDirectory
	}

	srcDir, err := filepath.Abs(relativeSrcDir)
	if err != nil {
		return fmt.Errorf("getting absolute path of %q: %w", relativeSrcDir, err)
	}

	projectName, err := promptProjectName(srcDir)
	if err != nil {
		return fmt.Errorf("running project name prompt: %w", err)
	}

	protocol, err := promptProtocol()
	if err != nil {
		return fmt.Errorf("running protocol prompt: %w", err)
	}

	switch protocol {
	case codegen.ProtocolEthereum:
		wantsABI, err := promptTrackContract()
		if err != nil {
			return fmt.Errorf("running ABI prompt: %w", err)
		}

		// Default 'Bored Ape Yacht Club' contract.
		// Used in 'github.com/streamingfast/substreams-template'
		contract := eth.MustNewAddress("bc4ca0eda7647a8ab7c2061c2e118a18a936f13d")

		if wantsABI {
			contract, err = promptEthereumVerifiedContract()
			if err != nil {
				return fmt.Errorf("running contract prompt: %w", err)
			}
		}

		// Get contract abiContent & parse
		abiContent, abi, err := codegen.GetContractABI(cmd.Context(), contract)
		if err != nil {
			return fmt.Errorf("getting contract ABI: %w", err)
		}

		// events, err := codegen.BuildEventModels(abi)
		// if err != nil {
		// 	return fmt.Errorf("build ABI event models: %w", err)
		// }

		project, err := templates.NewEthereumProject(projectName, templates.EthereumChainsByID["ethereum_mainnet"], contract, abi, abiContent)
		if err != nil {
			return fmt.Errorf("new ethereum project: %w", err)
		}

		files, err := project.Render()
		if err != nil {
			return fmt.Errorf("render project: %w", err)
		}

		for relativeFile, content := range files {
			file := path.Join(srcDir, strings.ReplaceAll(relativeFile, "/", string(os.PathSeparator)))

			directory := path.Dir(file)
			if err := os.MkdirAll(directory, os.ModePerm); err != nil {
				return fmt.Errorf("create directory %q: %w", directory, err)
			}

			if err := os.WriteFile(file, content, os.ModePerm); err != nil {
				return fmt.Errorf("write file: %w", err)
			}
		}

		// err = codegen.NewProjectGenerator(srcDir, projectName, contract, string(abiContent), events).GenerateProject()
		// if err != nil {
		// 	return fmt.Errorf("generating code: %w", err)
		// }

	case codegen.ProtocolOther:
		fmt.Println()
		fmt.Println("We haven't added any templates for your selected chain quite yet...")
		fmt.Println()
		fmt.Println("Come join us in discord at https://discord.gg/u8amUbGBgF and suggest templates/chains you want to see!")
		fmt.Println()

		return errInitUnsupportedChain
	}

	fmt.Printf("Project %q initialized at %q\n", projectName, srcDir)

	return nil
}

// We accept _ here because they are used across developers. we sanitize it later when
// used within Substreams module.
var moduleNameRegexp = regexp.MustCompile(`^([a-zA-Z][a-zA-Z0-9_-]{0,63})$`)

func promptProjectName(absoluteSrcDir string) (string, error) {
	if devInitProjectName != "" {
		return devInitProjectName, nil
	}

	return prompt("Project name", &promptOptions{
		Validate: func(input string) error {
			ok := moduleNameRegexp.MatchString(input)
			if !ok {
				return fmt.Errorf("invalid name: must match %s", moduleNameRegexp)
			}

			if cli.DirectoryExists(input) {
				return fmt.Errorf("project %q already exist in %q", input, absoluteSrcDir)
			}

			return nil
		},
	})
}

func promptEthereumVerifiedContract() (eth.Address, error) {
	if devInitEthereumTrackedContract != "" {
		// It's ok to panic, we expect the dev to put in a valid Ethereum address
		return eth.MustNewAddress(devInitEthereumTrackedContract), nil
	}

	return promptT("Verified Ethereum mainnet contract to track", eth.NewAddress, &promptOptions{
		Validate: func(input string) error {
			_, err := eth.NewAddress(input)
			if err != nil {
				return fmt.Errorf("invalid address: %w", err)
			}

			return nil
		},
	})
}

func promptTrackContract() (bool, error) {
	if devInitEthereumTrackedContract != "" {
		return true, nil
	}

	return promptConfirm("Would you like to track a particular contract", &promptOptions{
		PromptTemplates: &promptui.PromptTemplates{
			Success: `{{ "Track contract:" | faint }} `,
		},
	})
}

func promptProtocol() (codegen.Protocol, error) {
	if devInitProtocol != "" {
		// It's ok to panic, we expect the dev to put in a valid Ethereum address
		protocol, err := codegen.ParseProtocol(devInitProtocol)
		if err != nil {
			panic(fmt.Errorf("invalid protocol: %w", err))
		}

		return protocol, nil
	}

	choice := promptui.Select{
		Label: "Select protocol",
		Items: codegen.ProtocolNames(),
		Templates: &promptui.SelectTemplates{
			Selected: `{{ "Protocol:" | faint }} {{ . }}`,
		},
		HideHelp: true,
	}

	_, selection, err := choice.Run()
	if err != nil {
		if errors.Is(err, promptui.ErrInterrupt) {
			// We received Ctrl-C, users wants to abort, nothing else to do, quit immediately
			os.Exit(1)
		}

		return codegen.ProtocolOther, fmt.Errorf("running protocol prompt: %w", err)
	}

	var protocol codegen.Protocol
	if err := protocol.UnmarshalText([]byte(selection)); err != nil {
		panic(fmt.Errorf("impossible, selecting hard-coded value from enum itself, something is really wrong here"))
	}

	return protocol, nil
}

type promptOptions struct {
	Validate        promptui.ValidateFunc
	IsConfirm       bool
	PromptTemplates *promptui.PromptTemplates
}

var confirmPromptRegex = regexp.MustCompile("(y|Y|n|N|No|Yes|YES|NO)")

func prompt(label string, opts *promptOptions) (string, error) {
	var templates *promptui.PromptTemplates

	if opts != nil {
		templates = opts.PromptTemplates
	}

	if templates == nil {
		templates = &promptui.PromptTemplates{
			Success: `{{ . | faint }}{{ ":" | faint}} `,
		}
	}

	if opts != nil && opts.IsConfirm {
		// We don't have no differences
		templates.Valid = `{{ "?" | blue}} {{ . | bold }} {{ "[y/N]" | faint}} `
		templates.Invalid = templates.Valid
	}

	prompt := promptui.Prompt{
		Label:     label,
		Templates: templates,
	}
	if opts != nil && opts.Validate != nil {
		prompt.Validate = opts.Validate
	}

	if opts != nil && opts.IsConfirm {
		prompt.Validate = func(in string) error {
			if !confirmPromptRegex.MatchString(in) {
				return errors.New("answer with y/yes/Yes or n/no/No")
			}

			return nil
		}
	}

	choice, err := prompt.Run()
	if err != nil {
		if errors.Is(err, promptui.ErrInterrupt) {
			// We received Ctrl-C, users wants to abort, nothing else to do, quit immediately
			os.Exit(1)
		}

		if prompt.IsConfirm && errors.Is(err, promptui.ErrAbort) {
			return "false", nil
		}

		return "", fmt.Errorf("running prompt: %w", err)
	}

	return choice, nil
}

// promptT is just like [prompt] but accepts a transformer that transform the `string` into the generic type T.
func promptT[T any](label string, transformer func(string) (T, error), opts *promptOptions) (T, error) {
	choice, err := prompt(label, opts)
	if err == nil {
		return transformer(choice)
	}

	var empty T
	return empty, err
}

// promptConfirm is just like [prompt] but enforce `IsConfirm` and returns a boolean which is either
// `true` for yes answer or `false` for a no answer.
func promptConfirm(label string, opts *promptOptions) (bool, error) {
	if opts == nil {
		opts = &promptOptions{}
	}

	opts.IsConfirm = true
	transform := func(in string) (bool, error) {
		in = strings.ToLower(in)
		return in == "y" || in == "yes", nil
	}

	return promptT(label, transform, opts)
}
