package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"os/exec"
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
	devInitSinkChoice              = os.Getenv("SUBSTREAMS_DEV_INIT_SINK_CHOICE")
)

var errInitUnsupportedChain = errors.New("unsupported chain")
var errInitUnsupportedProtocol = errors.New("unsupported protocol")

var initCmd = &cobra.Command{
	Use:   "init [<path>]",
	Short: "Initialize a new, working Substreams project from scratch.",
	Long: cli.Dedent(`
		Initialize a new, working Substreams project from scratch. The path parameter is optional,
		with your current working directory being the default value.
        If you have an Etherscan API Key, you can set it to "ETHERSCAN_API_KEY" environment variable, it will be used to fetch the ABIs and contract information.
	`),
	RunE:         runSubstreamsInitE,
	Args:         cobra.RangeArgs(0, 1),
	SilenceUsage: true,
}

var etherscanAPIKey = "YourApiKeyToken"

func init() {
	if x := os.Getenv("ETHERSCAN_API_KEY"); x != "" {
		etherscanAPIKey = x
	}
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

	absoluteProjectDir := filepath.Join(absoluteWorkingDir, projectName)

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

		ethereumContracts, err := promptEthereumVerifiedContracts(eth.MustNewAddress(chain.DefaultContractAddress), chain.DefaultContractName)
		if err != nil {
			return fmt.Errorf("running contract prompt: %w", err)
		}

		if len(ethereumContracts) != 1 {
			// more than one contract to track, need to set the short names of the contracts
			fmt.Printf("Tracking %d contracts, let's define a short name for each contract\n", len(ethereumContracts))
			ethereumContracts, err = promptEthereumContractShortNames(ethereumContracts)
			if err != nil {
				return fmt.Errorf("running short name contract prompt: %w", err)
			}
		}

		fmt.Printf("Retrieving %s contract information (ABI & creation block)\n", chain.DisplayName)

		// Get contract abiContents & parse them
		ethereumContracts, err = getAndSetContractABIs(cmd.Context(), ethereumContracts, chain)
		if err != nil {
			return fmt.Errorf("getting %s contract ABI: %w", chain.DisplayName, err)
		}

		// Get contract creation block
		lowestStartBlock, err := getContractCreationBlock(cmd.Context(), ethereumContracts, chain)
		if err != nil {
			// FIXME: not sure if we should simplify set the contract block num to zero by default
			return fmt.Errorf("getting %s contract creating block: %w", chain.DisplayName, err)
		}

		for _, contract := range ethereumContracts {
			fmt.Printf("Generating ABI Event models for %s\n", contract.GetName())
			events, err := templates.BuildEventModels(contract, len(ethereumContracts) > 1)
			if err != nil {
				return fmt.Errorf("build ABI event models for contract [%s - %s]: %w", contract.GetAddress(), contract.GetName(), err)
			}
			contract.SetEvents(events)
		}

		fmt.Println("Writing project files")
		project, err := templates.NewEthereumProject(
			projectName,
			moduleName,
			chain,
			ethereumContracts,
			lowestStartBlock,
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
	fmt.Println()
	fmt.Println("Run 'make build' to build the wasm code.")
	fmt.Println()
	fmt.Println("The following substreams.yaml files have been created with different sink targets:")
	fmt.Println(" * substreams.yaml: no sink target")
	fmt.Println(" * substreams.sql.yaml: PostgreSQL sink")
	fmt.Println(" * substreams.clickhouse.yaml: Clickhouse sink")
	fmt.Println(" * substreams.subgraph.yaml: Sink into Substreams-based subgraph")

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
		file := filepath.Join(absoluteProjectDir, strings.ReplaceAll(relativeFile, "/", string(os.PathSeparator)))

		directory := filepath.Dir(file)
		if err := os.MkdirAll(directory, os.ModePerm); err != nil {
			return fmt.Errorf("create directory %q: %w", directory, err)
		}

		if err := os.WriteFile(file, content, 0644); err != nil {
			// remove directory, we want a complete e2e file generation
			e := os.RemoveAll(directory)
			if e != nil {
				return fmt.Errorf("removing directory %s: %w and write file: %w", directory, e, err)
			}
			return fmt.Errorf("write file: %w", err)
		}
	}

	return nil
}

// We accept _ here because they are used across developers. we sanitize it later when
// used within Substreams module.
var moduleNameRegexp = regexp.MustCompile(`^([a-z][a-z0-9_]{0,63})$`)

func promptProjectName(absoluteSrcDir string) (string, string, error) {
	if name := devInitProjectName; name != "" {
		return name, projectNameToModuleName(name), nil
	}

	projectName, err := prompt("Project name (lowercase, numbers, undescores)", &promptOptions{
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

func promptEthereumVerifiedContracts(defaultAddress eth.Address, defaultContractName string) ([]*templates.EthereumContract, error) {
	if devInitEthereumTrackedContract != "" {
		// It's ok to panic, we expect the dev to put in a valid Ethereum address
		return []*templates.EthereumContract{
			templates.NewEthereumContract("", eth.MustNewAddress(devInitEthereumTrackedContract), nil, nil, ""),
		}, nil
	}

	var ethContracts []*templates.EthereumContract

	inputOrDefaultFunc := func(input string) (eth.Address, error) {
		if input == "" {
			return defaultAddress, nil
		}
		return eth.NewAddress(input)
	}

	inputOrEmptyFunc := func(input string) (eth.Address, error) {
		if input == "" {
			return nil, nil
		}
		return eth.NewAddress(input)
	}

	firstContractAddress, err := promptContractAddress(fmt.Sprintf("Contract address to track (leave empty to use %q)", defaultContractName), inputOrDefaultFunc)
	if err != nil {
		return nil, err
	}

	ethContracts = append(ethContracts, templates.NewEthereumContract("", firstContractAddress, nil, nil, ""))

	if bytes.Equal(firstContractAddress, defaultAddress) {
		return ethContracts, nil
	}

	for {
		contractAddr, err := promptContractAddress("Would you like to track another contract? (Leave empty if not)", inputOrEmptyFunc)
		if err != nil {
			return nil, err
		}

		if contractAddr == nil {
			return ethContracts, nil
		}
		ethContracts = append(ethContracts, templates.NewEthereumContract("", contractAddr, nil, nil, ""))
	}
}

func promptContractAddress(message string, inputFuncCheck func(input string) (eth.Address, error)) (eth.Address, error) {
	return promptT(message, inputFuncCheck, &promptOptions{
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

var shortNameRegexp = regexp.MustCompile(`^([a-z][a-z0-9]{0,63})$`)

func promptEthereumContractShortNames(ethereumContracts []*templates.EthereumContract) ([]*templates.EthereumContract, error) {
	for _, contract := range ethereumContracts {
		shortName, err := prompt(fmt.Sprintf("Choose a short name for %s (lowercase and numbers only)", contract.GetAddress()), &promptOptions{
			Validate: func(input string) error {
				ok := shortNameRegexp.MatchString(input)
				if !ok {
					return fmt.Errorf("invalid name: must match %s", shortNameRegexp)
				}

				return nil
			},
		})
		if err != nil {
			return nil, err
		}
		contract.SetName(shortName)
	}

	return ethereumContracts, nil
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

// getProxyContractImplementation returns the implementation address and a timer to wait before next call
func getProxyContractImplementation(ctx context.Context, address eth.Address, endpoint string) (*eth.Address, *time.Timer, error) {
	// check for proxy contract's implementation
	req, err := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("%s/api?module=contract&action=getsourcecode&address=%s&apiKey=%s", endpoint, address.Pretty(), etherscanAPIKey), nil)

	if err != nil {
		return nil, nil, fmt.Errorf("new request: %w", err)
	}

	res, err := httpClient.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("getting contract abi from etherscan: %w", err)
	}
	defer res.Body.Close()

	type Response struct {
		Message string `json:"message"` // ex: `OK-Missing/Invalid API Key, rate limit of 1/5sec applied`
		Result  []struct {
			Implementation string `json:"Implementation"`
			// ContractName string `json:"ContractName"`
		} `json:"result"`
	}

	var response Response

	bod, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, nil, err
	}
	if err := json.NewDecoder(bytes.NewReader(bod)).Decode(&response); err != nil {
		return nil, nil, fmt.Errorf("unmarshaling %s: %w", string(bod), err)
	}

	timer := timerUntilNextCall(response.Message)

	if len(response.Result) == 0 {
		return nil, timer, nil
	}

	if len(response.Result[0].Implementation) != 42 {
		return nil, timer, nil
	}

	addr, err := eth.NewAddress(response.Result[0].Implementation)
	if err != nil {
		return nil, timer, err
	}
	return &addr, timer, nil
}

func timerUntilNextCall(msg string) *time.Timer {
	// etherscan-specific
	if strings.HasPrefix(msg, "OK-Missing/Invalid API Key") {
		return time.NewTimer(time.Second * 5)
	}
	return time.NewTimer(time.Millisecond * 400)
}

func getContractABI(ctx context.Context, address eth.Address, endpoint string) (*eth.ABI, string, *time.Timer, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("%s/api?module=contract&action=getabi&address=%s&apiKey=%s", endpoint, address.Pretty(), etherscanAPIKey), nil)
	if err != nil {
		return nil, "", nil, fmt.Errorf("new request: %w", err)
	}

	res, err := httpClient.Do(req)
	if err != nil {
		return nil, "", nil, fmt.Errorf("getting contract abi from etherscan: %w", err)
	}
	defer res.Body.Close()

	type Response struct {
		Message string      `json:"message"` // ex: `OK-Missing/Invalid API Key, rate limit of 1/5sec applied`
		Result  interface{} `json:"result"`
	}

	var response Response
	if err := json.NewDecoder(res.Body).Decode(&response); err != nil {
		return nil, "", nil, fmt.Errorf("unmarshaling: %w", err)
	}

	timer := timerUntilNextCall(response.Message)

	abiContent, ok := response.Result.(string)
	if !ok {
		return nil, "", timer, fmt.Errorf(`invalid response "Result" field type, expected "string" got "%T"`, response.Result)
	}

	ethABI, err := eth.ParseABIFromBytes([]byte(abiContent))
	if err != nil {
		return nil, "", timer, fmt.Errorf("parsing abi %q: %w", abiContent, err)
	}
	return ethABI, abiContent, timer, err
}

func getAndSetContractABIs(ctx context.Context, contracts []*templates.EthereumContract, chain *templates.EthereumChain) ([]*templates.EthereumContract, error) {
	for _, contract := range contracts {
		abi, abiContent, wait, err := getContractABI(ctx, contract.GetAddress(), chain.ApiEndpoint)
		if err != nil {
			return nil, err
		}

		<-wait.C
		implementationAddress, wait, err := getProxyContractImplementation(ctx, contract.GetAddress(), chain.ApiEndpoint)
		if err != nil {
			return nil, err
		}
		<-wait.C

		if implementationAddress != nil {
			implementationABI, implementationABIContent, wait, err := getContractABI(ctx, *implementationAddress, chain.ApiEndpoint)
			if err != nil {
				return nil, err
			}
			for k, v := range implementationABI.LogEventsMap {
				abi.LogEventsMap[k] = append(abi.LogEventsMap[k], v...)
			}

			for k, v := range implementationABI.LogEventsByNameMap {
				abi.LogEventsByNameMap[k] = append(abi.LogEventsByNameMap[k], v...)
			}

			abiAsArray := []map[string]interface{}{}
			if err := json.Unmarshal([]byte(abiContent), &abiAsArray); err != nil {
				return nil, fmt.Errorf("unmarshalling abiContent as array: %w", err)
			}

			implementationABIAsArray := []map[string]interface{}{}
			if err := json.Unmarshal([]byte(implementationABIContent), &implementationABIAsArray); err != nil {
				return nil, fmt.Errorf("unmarshalling implementationABIContent as array: %w", err)
			}

			abiAsArray = append(abiAsArray, implementationABIAsArray...)

			content, err := json.Marshal(abiAsArray)
			if err != nil {
				return nil, fmt.Errorf("re-marshalling ABI")
			}
			abiContent = string(content)

			fmt.Printf("Fetched contract ABI for Implementation %s of Proxy %s\n", *implementationAddress, contract.GetAddress())
			<-wait.C
		}

		//fmt.Println("this is the complete abiContent after merge", abiContent)
		contract.SetAbiContent(abiContent)
		contract.SetAbi(abi)

		fmt.Printf("Fetched contract ABI for %s\n", contract.GetAddress())
	}

	return contracts, nil
}

func getContractCreationBlock(ctx context.Context, contracts []*templates.EthereumContract, chain *templates.EthereumChain) (uint64, error) {
	var lowestStartBlock uint64 = math.MaxUint64
	for _, contract := range contracts {
		req, err := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("%s/api?module=account&action=txlist&address=%s&page=1&offset=1&sort=asc&apikey=%s", chain.ApiEndpoint, contract.GetAddress().Pretty(), etherscanAPIKey), nil)
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

		<-timerUntilNextCall(response.Message).C

		blockNum, err := strconv.ParseUint(response.Result[0].BlockNumber, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("parsing block number: %w", err)
		}

		if blockNum < lowestStartBlock {
			lowestStartBlock = blockNum
		}

		fmt.Printf("Fetched initial block %d for %s (lowest %d)\n", blockNum, contract.GetAddress(), lowestStartBlock)
	}
	return lowestStartBlock, nil
}
