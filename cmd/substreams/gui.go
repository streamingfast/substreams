package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
	"github.com/streamingfast/cli"
	"github.com/streamingfast/cli/sflags"
	"github.com/streamingfast/substreams/client"
	"github.com/streamingfast/substreams/manifest"
	"github.com/streamingfast/substreams/tools"
	"github.com/streamingfast/substreams/tui2"
	"github.com/streamingfast/substreams/tui2/pages/request"
)

func init() {
	guiCmd.Flags().String("substreams-api-token-envvar", "SUBSTREAMS_API_TOKEN", "name of variable containing Substreams Authentication token")
	guiCmd.Flags().String("substreams-api-key-envvar", "SUBSTREAMS_API_KEY", "Name of variable containing Substreams Api Key")
	guiCmd.Flags().StringP("substreams-endpoint", "e", "", "Substreams gRPC endpoint. If empty, will be replaced by the SUBSTREAMS_ENDPOINT_{network_name} environment variable, where `network_name` is determined from the substreams manifest. Some network names have default endpoints.")
	guiCmd.Flags().String("network", "", "Specify the network to use for params and initialBlocks, overriding the 'network' field in the substreams package")
	guiCmd.Flags().Bool("insecure", false, "Skip certificate validation on GRPC connection")
	guiCmd.Flags().Bool("plaintext", false, "Establish GRPC connection in plaintext")
	guiCmd.Flags().StringSliceP("header", "H", nil, "Additional headers to be sent in the substreams request")
	guiCmd.Flags().StringP("start-block", "s", "", "Start block to stream from. If empty, will be replaced by initialBlock of the first module you are streaming. If negative, will be resolved by the server relative to the chain head")
	guiCmd.Flags().StringP("cursor", "c", "", "Cursor to stream from. Leave blank for no cursor")
	guiCmd.Flags().StringP("stop-block", "t", "0", "Stop block to end stream at, inclusively.")
	guiCmd.Flags().Bool("final-blocks-only", false, "Only process blocks that have pass finality, to prevent any reorg and undo signal by staying further away from the chain HEAD")
	guiCmd.Flags().StringSlice("debug-modules-initial-snapshot", nil, "List of 'store' modules from which to print the initial data snapshot (Unavailable in Production Mode")
	guiCmd.Flags().StringSlice("debug-modules-output", nil, "List of extra modules from which to print outputs, deltas and logs (Unavailable in Production Mode)")
	guiCmd.Flags().Bool("production-mode", false, "Enable Production Mode, with high-speed parallel processing")
	guiCmd.Flags().StringArrayP("params", "p", nil, "Set a params for parameterizable modules. Can be specified multiple times. Ex: -p module1=valA -p module2=valX&valY")
	guiCmd.Flags().Bool("replay", false, "Replay saved session into GUI from replay.bin")
	guiCmd.Flags().Bool("skip-package-validation", false, "Do not perform any validation when reading substreams package")
	rootCmd.AddCommand(guiCmd)
}

// guiCmd represents the command to run substreams remotely
var guiCmd = &cobra.Command{
	Use:   "gui [<manifest>] <module_name>",
	Short: "Stream module outputs from a given package on a remote endpoint",
	Long: cli.Dedent(`
		Stream module outputs from a given package on a remote endpoint. The manifest is optional as it will try to find a file named
		'substreams.yaml' in current working directory if nothing entered. You may enter a directory that contains a 'substreams.yaml'
		file in place of '<manifest_file>, or a link to a remote .spkg file, using urls gs://, http(s)://, ipfs://, etc.'.
	`),
	RunE:         runGui,
	Args:         cobra.RangeArgs(1, 2),
	SilenceUsage: true,
}

func runGui(cmd *cobra.Command, args []string) error {
	// TODO: DRY up this and `run` .. such duplication here.

	manifestPath := ""
	var err error
	if len(args) == 2 {
		manifestPath = args[0]
		args = args[1:]
	} else {
		// Check common error where manifest is provided by module name is missing
		if manifest.IsLikelyManifestInput(args[0]) {
			return fmt.Errorf("missing <module_name> argument, check 'substreams run --help' for more information")
		}

		// At this point, we assume the user invoked `substreams run <module_name>` so we `resolveManifestFile` using the empty string since no argument has been passed.
		manifestPath, err = resolveManifestFile("")
		if err != nil {
			return fmt.Errorf("resolving manifest: %w", err)
		}
	}

	productionMode := mustGetBool(cmd, "production-mode")
	debugModulesOutput := mustGetStringSlice(cmd, "debug-modules-output")
	if debugModulesOutput != nil && productionMode {
		return fmt.Errorf("cannot set 'debug-modules-output' in 'production-mode'")
	}
	debugModulesInitialSnapshot := mustGetStringSlice(cmd, "debug-modules-initial-snapshot")

	outputModule := args[0]
	network := sflags.MustGetString(cmd, "network")
	paramsString := sflags.MustGetStringArray(cmd, "params")
	params, err := manifest.ParseParams(paramsString)
	if err != nil {
		return fmt.Errorf("parsing params: %w", err)
	}

	readerOptions := []manifest.Option{
		manifest.WithOverrideOutputModule(outputModule),
		manifest.WithOverrideNetwork(network),
		manifest.WithParams(params),
	}
	if sflags.MustGetBool(cmd, "skip-package-validation") {
		readerOptions = append(readerOptions, manifest.SkipPackageValidationReader())
	}

	manifestReader, err := manifest.NewReader(manifestPath, readerOptions...)
	if err != nil {
		return fmt.Errorf("manifest reader: %w", err)
	}

	pkg, graph, err := manifestReader.Read()
	if err != nil {
		return fmt.Errorf("read manifest %q: %w", manifestPath, err)
	}

	endpoint, err := manifest.ExtractNetworkEndpoint(pkg.Network, mustGetString(cmd, "substreams-endpoint"), zlog)
	if err != nil {
		return fmt.Errorf("extracting endpoint: %w", err)
	}

	authToken, authType := tools.GetAuth(cmd, "substreams-api-key-envvar", "substreams-api-token-envvar")
	substreamsClientConfig := client.NewSubstreamsClientConfig(
		endpoint,
		authToken,
		authType,
		mustGetBool(cmd, "insecure"),
		mustGetBool(cmd, "plaintext"),
	)

	homeDir, err := os.UserHomeDir()
	if err != nil {
		homeDir = "."
	} else {
		err = os.MkdirAll(filepath.Join(homeDir, ".config", "substreams"), 0755)
		if err != nil {
			return fmt.Errorf("creating config directory: %w", err)
		}

		homeDir = filepath.Join(homeDir, ".config", "substreams")
	}

	cursor := mustGetString(cmd, "cursor")

	fmt.Println("Launching Substreams GUI...")

	startBlock, readFromModule, err := readStartBlockFlag(cmd, "start-block")
	if err != nil {
		return fmt.Errorf("start block: %w", err)
	}

	stopBlock, err := readStopBlockFlag(cmd, startBlock, "stop-block", cursor != "")
	if err != nil {
		return fmt.Errorf("stop block: %w", err)
	}

	if readFromModule { // need to tweak the stop block here
		sb, err := graph.ModuleInitialBlock(outputModule)
		if err != nil {
			return fmt.Errorf("getting module start block: %w", err)
		}
		startBlock := int64(sb)
		stopBlock, err = readStopBlockFlag(cmd, startBlock, "stop-block", cursor != "")
		if err != nil {
			return fmt.Errorf("stop block: %w", err)
		}
	}

	requestConfig := &request.Config{
		ManifestPath:                manifestPath,
		Pkg:                         pkg,
		Graph:                       graph,
		ReadFromModule:              readFromModule,
		ProdMode:                    productionMode,
		DebugModulesOutput:          debugModulesOutput,
		DebugModulesInitialSnapshot: debugModulesInitialSnapshot,
		OutputModule:                outputModule,
		SubstreamsClientConfig:      substreamsClientConfig,
		HomeDir:                     homeDir,
		Vcr:                         mustGetBool(cmd, "replay"),
		Headers:                     parseHeaders(mustGetStringSlice(cmd, "header")),
		Cursor:                      cursor,
		StartBlock:                  startBlock,
		StopBlock:                   stopBlock,
		FinalBlocksOnly:             mustGetBool(cmd, "final-blocks-only"),
		Params:                      params,
		ReaderOptions:               readerOptions,
	}

	ui, err := tui2.New(requestConfig)
	if err != nil {
		return err
	}
	prog := tea.NewProgram(ui, tea.WithAltScreen())
	if _, err := prog.Run(); err != nil {
		return fmt.Errorf("gui error: %w", err)
	}

	return nil
}

// resolveManifestFile is solely nowadays by `substreams gui`. That is because manifest.Reader
// now has the ability to resolve itself to the correct location.
//
// However `substreams gui` displays the value, so we want to display the resolved
// value to the user.
//
// FIXME: Find a way to share this with manifest.Reader somehow. Maybe as a method on
// on the reader which would resolve the file, sharing the internal logic.
func resolveManifestFile(input string) (manifestName string, err error) {
	if input == "" {
		_, err := os.Stat("substreams.yaml")
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return "", fmt.Errorf("no manifest entered in directory without a manifest")
			}
			return "", fmt.Errorf("finding manifest: %w", err)
		}

		return "substreams.yaml", nil
	} else if strings.HasSuffix(input, ".spkg") {
		return input, nil
	}

	inputInfo, err := os.Stat(input)
	if err != nil {
		return "", fmt.Errorf("read input file info: %w", err)
	}

	if inputInfo.IsDir() {
		potentialManifest := filepath.Join(inputInfo.Name(), "substreams.yaml")
		_, err := os.Stat(potentialManifest)
		if err != nil {
			return "", fmt.Errorf("finding manifest in directory: %w", err)
		}
		return filepath.Join(input, "substreams.yaml"), nil
	}
	return input, nil
}
