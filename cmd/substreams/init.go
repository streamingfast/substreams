package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/manifoldco/promptui"
	"github.com/spf13/cobra"
	"github.com/streamingfast/cli"
	"github.com/streamingfast/dhttp"
	"github.com/streamingfast/eth-go"
	"github.com/streamingfast/substreams/codegen"
	"github.com/streamingfast/substreams/codegen/templates"
)

// Some developers centric environment override to make it faster to iterate on `substreams init` command
var (
	devInitSourceDirectory         = os.Getenv("SUBSTREAMS_DEV_INIT_SOURCE_DIRECTORY")
	devInitProjectName             = os.Getenv("SUBSTREAMS_DEV_INIT_PROJECT_NAME")
	devInitProtocol                = os.Getenv("SUBSTREAMS_DEV_INIT_PROTOCOL")
	devInitEthereumTrackedContract = os.Getenv("SUBSTREAMS_DEV_INIT_ETHEREUM_TRACKED_CONTRACT")
	devInitEthereumChain           = os.Getenv("SUBSTREAMS_DEV_INIT_ETHEREUM_CHAIN")
)

var errInitUnsupportedChain = errors.New("unsupported chain")
var errInitUnsupportedProtocol = errors.New("unsupported protocol")

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
	rootCmd.AddCommand(initCmd)
}

func runSubstreamsInitE(cmd *cobra.Command, args []string) error {
	relativeWorkingDir := "."
	if len(args) == 1 {
		relativeWorkingDir = args[0]
	}

	if devInitSourceDirectory != "" {
		relativeWorkingDir = devInitSourceDirectory
	}

	absoluteWorkingDir, err := filepath.Abs(relativeWorkingDir)
	if err != nil {
		return fmt.Errorf("getting absolute path of %q: %w", relativeWorkingDir, err)
	}

	projectName, moduleName, err := promptProjectName(absoluteWorkingDir)
	if err != nil {
		return fmt.Errorf("running project name prompt: %w", err)
	}

	absoluteProjectDir := path.Join(absoluteWorkingDir, projectName)

	protocol, err := promptProtocol()
	if err != nil {
		return fmt.Errorf("running protocol prompt: %w", err)
	}

	switch protocol {
	case codegen.ProtocolEthereum:
		chainSelected, err := promptEthereumChain()
		if err != nil {
			return fmt.Errorf("running chain prompt: %w", err)
		}
		if chainSelected == codegen.EthereumChainOther {
			fmt.Println()
			fmt.Println("We haven't added any templates for your selected chain quite yet")
			fmt.Println()
			fmt.Println("Come join us in discord at https://discord.gg/u8amUbGBgF and suggest templates/chains you want to see!")
			fmt.Println()
			return errInitUnsupportedChain
		}

		chain := templates.EthereumChainsByID[chainSelected.String()]
		if chain == nil {
			return fmt.Errorf("unknown chain: %s", chainSelected.String())
		}

		contract := eth.MustNewAddress(chain.DefaultContractAddress)

		contract, err = promptEthereumVerifiedContract(contract, chain.DefaultContractName)
		if err != nil {
			return fmt.Errorf("running contract prompt: %w", err)
		}

		fmt.Printf("Retrieving %s contract information (ABI & creation block)\n", chain.DisplayName)

		// Get contract abiContent & parse
		abiContent, abi, err := getContractABI(cmd.Context(), contract, chain)
		if err != nil {
			return fmt.Errorf("getting %s contract ABI: %w", chain.DisplayName, err)
		}

		// Get contract creation block
		// First, wait 5 seconds to avoid Etherscan API rate limit
		time.Sleep(5 * time.Second)
		creationBlockNum, err := getContractCreationBlock(cmd.Context(), contract, chain)
		if err != nil {
			fmt.Printf("getting %s contract creation block, using 0 instead: %v\n", chain.DisplayName, err)
			creationBlockNum = 0
		}

		fmt.Println("Writing project files")
		project, err := templates.NewEthereumProject(
			projectName,
			moduleName,
			chain,
			contract,
			abi,
			abiContent,
			creationBlockNum,
		)
		if err != nil {
			return fmt.Errorf("new Ethereum %s project: %w", chain.DisplayName, err)
		}

		if err := renderProjectFilesIn(project, absoluteProjectDir); err != nil {
			return fmt.Errorf("render Ethereum %s project: %w", chain.DisplayName, err)
		}

	case codegen.ProtocolOther:
		fmt.Println()
		fmt.Println("We haven't added any templates for your selected protocol quite yet")
		fmt.Println()
		fmt.Println("Come join us in discord at https://discord.gg/u8amUbGBgF and suggest templates/chains you want to see!")
		fmt.Println()

		return errInitUnsupportedProtocol
	}

	fmt.Println("Generating Protobuf Rust code")
	if err := protogenSubstreams(absoluteProjectDir); err != nil {
		return fmt.Errorf("protobuf generation: %w", err)
	}

	fmt.Printf("Project %q initialized at %q\n", projectName, absoluteWorkingDir)

	return nil
}

func protogenSubstreams(absoluteProjectDir string) error {
	cmd := exec.Command("substreams", "protogen", `--exclude-paths`, `sf/substreams,google`)
	cmd.Dir = absoluteProjectDir

	output, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Println(string(output))
		return fmt.Errorf("running %q failed: %w", cmd, err)
	}

	return nil
}

func renderProjectFilesIn(project templates.Project, absoluteProjectDir string) error {
	files, err := project.Render()
	if err != nil {
		return fmt.Errorf("render project: %w", err)
	}

	for relativeFile, content := range files {
		file := path.Join(absoluteProjectDir, strings.ReplaceAll(relativeFile, "/", string(os.PathSeparator)))

		directory := path.Dir(file)
		if err := os.MkdirAll(directory, os.ModePerm); err != nil {
			return fmt.Errorf("create directory %q: %w", directory, err)
		}

		if err := os.WriteFile(file, content, os.ModePerm); err != nil {
			return fmt.Errorf("write file: %w", err)
		}
	}

	return nil
}

// We accept _ here because they are used across developers. we sanitize it later when
// used within Substreams module.
var moduleNameRegexp = regexp.MustCompile(`^([a-zA-Z][a-zA-Z0-9_-]{0,63})$`)

func promptProjectName(absoluteSrcDir string) (string, string, error) {
	if name := devInitProjectName; name != "" {
		return name, projectNameToModuleName(name), nil
	}

	projectName, err := prompt("Project name", &promptOptions{
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
	if err != nil {
		return "", "", err
	}

	return projectName, projectNameToModuleName(projectName), nil
}

func projectNameToModuleName(in string) string {
	return strings.ReplaceAll(in, "-", "_")
}

func promptEthereumVerifiedContract(defaultAddress eth.Address, defaultContractName string) (eth.Address, error) {
	if devInitEthereumTrackedContract != "" {
		// It's ok to panic, we expect the dev to put in a valid Ethereum address
		return eth.MustNewAddress(devInitEthereumTrackedContract), nil
	}

	inputOrDefaultFunc := func(input string) (eth.Address, error) {
		if input == "" {
			return defaultAddress, nil
		}
		return eth.NewAddress(input)
	}

	return promptT(fmt.Sprintf("Contract address to track (leave empty to use %q)", defaultContractName), inputOrDefaultFunc, &promptOptions{
		Validate: func(input string) error {
			if input == "" {
				return nil
			}
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
			Success: `{{ "Track particular contract:" | faint }} `,
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

func promptEthereumChain() (codegen.EthereumChain, error) {
	if devInitEthereumChain != "" {
		// It's ok to panic, we expect the dev to put in a valid Ethereum chain
		chain, err := codegen.ParseEthereumChain(devInitEthereumChain)
		if err != nil {
			panic(fmt.Errorf("invalid chain: %w", err))
		}

		return chain, nil
	}

	choice := promptui.Select{
		Label: "Select Ethereum chain",
		Items: codegen.EthereumChainNames(),
		Templates: &promptui.SelectTemplates{
			Selected: `{{ "Ethereum chain:" | faint }} {{ . }}`,
		},
		HideHelp: true,
	}

	_, selection, err := choice.Run()
	if err != nil {
		if errors.Is(err, promptui.ErrInterrupt) {
			// We received Ctrl-C, users wants to abort, nothing else to do, quit immediately
			os.Exit(1)
		}

		return codegen.EthereumChainOther, fmt.Errorf("running chain prompt: %w", err)
	}

	var chain codegen.EthereumChain
	if err := chain.UnmarshalText([]byte(selection)); err != nil {
		panic(fmt.Errorf("impossible, selecting hard-coded value from enum itself, something is really wrong here"))
	}

	return chain, nil
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
		// We have no differences
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

var httpClient = http.Client{
	Transport: dhttp.NewLoggingRoundTripper(zlog, tracer, http.DefaultTransport),
	Timeout:   30 * time.Second,
}

func getContractABI(ctx context.Context, contract eth.Address, chain *templates.EthereumChain) (string, *eth.ABI, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("%s/api?module=contract&action=getabi&address=%s&apikey=YourApiKeyToken", chain.ApiEndpoint, contract.Pretty()), nil)
	if err != nil {
		return "", nil, fmt.Errorf("new request: %w", err)
	}

	res, err := httpClient.Do(req)
	if err != nil {
		return "", nil, fmt.Errorf("getting contract abi from etherscan: %w", err)
	}
	defer res.Body.Close()

	type Response struct {
		Result interface{} `json:"result"`
	}

	var response Response
	if err := json.NewDecoder(res.Body).Decode(&response); err != nil {
		return "", nil, fmt.Errorf("unmarshaling: %w", err)
	}

	abiContent, ok := response.Result.(string)
	if !ok {
		return "", nil, fmt.Errorf(`invalid response "Result" field type, expected "string" got "%T"`, response.Result)
	}

	ethABI, err := eth.ParseABIFromBytes([]byte(abiContent))
	if err != nil {
		return "", nil, fmt.Errorf("parsing abi: %w", err)
	}

	return abiContent, ethABI, nil
}

func getContractCreationBlock(ctx context.Context, contract eth.Address, chain *templates.EthereumChain) (uint64, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("%s/api?module=account&action=txlist&address=%s&page=1&offset=1&sort=asc&apikey=YourApiKeyToken", chain.ApiEndpoint, contract.Pretty()), nil)
	if err != nil {
		return 0, fmt.Errorf("new request: %w", err)
	}

	res, err := httpClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("failed request to etherscan: %w", err)
	}
	defer res.Body.Close()

	type Response struct {
		Status  string `json:"status"`
		Message string `json:"message"`
		Result  []struct {
			BlockNumber string `json:"blockNumber"`
		} `json:"result"`
	}

	var response Response
	if err := json.NewDecoder(res.Body).Decode(&response); err != nil {
		return 0, fmt.Errorf("unmarshaling: %w", err)
	}

	if len(response.Result) == 0 {
		return 0, fmt.Errorf("empty result from response %v", response)
	}

	blockNum, err := strconv.ParseUint(response.Result[0].BlockNumber, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parsing block number: %w", err)
	}

	return blockNum, nil
}
