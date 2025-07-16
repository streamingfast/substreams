package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
	"github.com/streamingfast/cli/sflags"
	"github.com/streamingfast/substreams/sink"
	"github.com/streamingfast/substreams/tui2"
	"github.com/streamingfast/substreams/tui2/pages/request"
)

func init() {
	// Use sink's standard flags, but ignore some that don't apply to GUI
	sink.AddFlagsToSet(guiCmd.Flags(),
		sink.FlagIgnore(sink.FlagDevelopmentMode,
			sink.FlagLiveBlockTimeDelta,
			sink.FlagInfiniteRetry))

	// GUI-specific flags
	guiCmd.Flags().Bool("production-mode", false, "Enable Production Mode, with high-speed parallel processing")
	guiCmd.Flags().Uint64("limit-processed-blocks", 10000, "Limit the number of blocks to be processed by the server, including preparing the stores, as a safeguard to prevent unexpected expensive reprocessing (0 disables the limit)")
	guiCmd.Flags().StringSlice("debug-modules-initial-snapshot", nil, "List of 'store' modules from which to print the initial data snapshot (Unavailable in Production Mode)")
	guiCmd.Flags().StringSlice("debug-modules-output", nil, "List of modules from which to fetch the outputs, useful when debugging (unavailable in Production Mode). Defaults to the non-imported modules.")
	guiCmd.Flags().Bool("replay", false, "Replay saved session into GUI from replay.bin")

	rootCmd.AddCommand(guiCmd)
}

var guiOrRunLongUsage = `Stream module output from a given package on a remote endpoint. The manifest is optional as it will try to find a file named
'substreams.yaml' in current working directory if nothing entered. You may enter a directory that contains a 'substreams.yaml'
file in place of '<manifest_file>, or a link to a remote .spkg file, using urls gs://, http(s)://, ipfs://, etc.'.
You can also use "-" to read the manifest from standard input.

You can also use substreams gui my-package@v0.1.0 to specify a specific version of the package. This will fetch it from
the Substreams registry at https://substreams.dev`

// guiCmd represents the command to run substreams remotely
var guiCmd = &cobra.Command{
	Use:          "gui [<manifest> [<module_name>]]",
	Short:        "Open the GUI to stream module outputs",
	Long:         guiOrRunLongUsage,
	RunE:         runGui,
	Args:         cobra.RangeArgs(0, 2),
	SilenceUsage: true,
}

func ruiOrGuiManifestModulePositionalParams(args []string) (manifestPath string, outputModule string, err error) {
	switch len(args) {
	case 0:
		manifestPath, err = resolveManifestFile("")
		if err != nil {
			err = fmt.Errorf("resolving manifest: %w", err)
			return
		}
	case 1:
		manifestPath = args[0]
	case 2:
		manifestPath = args[0]
		outputModule = args[1]
	default:
		err = fmt.Errorf("too many arguments")
	}
	return
}

func runGui(cmd *cobra.Command, args []string) (err error) {
	manifestPath, outputModule, err := ruiOrGuiManifestModulePositionalParams(args)
	if err != nil {
		return err
	}

	// Load auth environment file if it exists
	loadSubstreamsAuthEnvFile(manifestPath)

	// Use sink pattern to create configuration
	sinkerConfig, err := sink.ConfigFromViper(cmd, sink.IgnoreOutputModuleType, manifestPath, outputModule, "substreams_gui", zlog, tracer)
	if err != nil {
		return fmt.Errorf("creating sink config: %w", err)
	}

	// Override production mode from our GUI-specific flag
	if sflags.MustGetBool(cmd, "production-mode") {
		sinkerConfig.Mode = sink.SubstreamsModeProduction
	} else {
		sinkerConfig.Mode = sink.SubstreamsModeDevelopment
	}

	// Set GUI-specific configuration
	sinkerConfig.LimitProcessedBlocks = sflags.MustGetUint64(cmd, "limit-processed-blocks")

	debugModulesOutput := sflags.MustGetStringSlice(cmd, "debug-modules-output")
	if len(debugModulesOutput) == 0 {
		debugModulesOutput = nil
	}
	sinkerConfig.DevOutputModules = debugModulesOutput

	debugModulesInitialSnapshot := sflags.MustGetStringSlice(cmd, "debug-modules-initial-snapshot")
	if len(debugModulesInitialSnapshot) == 0 {
		debugModulesInitialSnapshot = nil
	}
	sinkerConfig.DevOutputSnapshots = debugModulesInitialSnapshot

	// Validate production mode constraints
	if sinkerConfig.DevOutputModules != nil && sinkerConfig.Mode == sink.SubstreamsModeProduction {
		return fmt.Errorf("cannot set 'debug-modules-output' in 'production-mode'")
	}
	if sinkerConfig.DevOutputSnapshots != nil && sinkerConfig.Mode == sink.SubstreamsModeProduction {
		return fmt.Errorf("cannot set 'debug-modules-initial-snapshot' in 'production-mode'")
	}

	// Prepare home directory for GUI config
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

	// Extract default parameters from package
	defaultParams := sinkerConfig.ExtractDefaultParams()

	// Build GUI request configuration
	requestConfig := &request.Config{
		ManifestPath:                manifestPath,
		Pkg:                         sinkerConfig.Pkg,
		SkipPackageValidation:       sinkerConfig.SkipPackageValidation,
		Graph:                       nil, // Will be built from package
		ProdMode:                    sinkerConfig.Mode == sink.SubstreamsModeProduction,
		DebugModulesInitialSnapshot: sinkerConfig.DevOutputSnapshots,
		DebugModulesOutput:          sinkerConfig.DevOutputModules,
		Endpoint:                    sinkerConfig.ClientConfig.Endpoint(),
		OutputModule:                outputModule,
		OverrideNetwork:             sinkerConfig.Network,
		SubstreamsClientConfig:      sinkerConfig.ClientConfig,
		HomeDir:                     homeDir,
		Vcr:                         sflags.MustGetBool(cmd, "replay"),
		Headers:                     parseHeaders(sinkerConfig.ExtraHeaders),
		Cursor:                      "", // Will be set from flags
		StartBlock:                  fmt.Sprintf("%d", sinkerConfig.StartBlock),
		StopBlock:                   fmt.Sprintf("%d", sinkerConfig.StopBlock),
		FinalBlocksOnly:             sinkerConfig.FinalBlocksOnly,
		LimitProcessedBlocks:        sinkerConfig.LimitProcessedBlocks,
		Params:                      strings.Join(sinkerConfig.Params, "\n"),
		DefaultParams:               strings.Join(defaultParams, "\n"),
	}

	// Set cursor from flags if provided
	if cursorFlag := sflags.MustGetString(cmd, sink.FlagCursor); cursorFlag != "" {
		requestConfig.Cursor = cursorFlag
	}

	if err := requestConfig.Normalize(); err != nil {
		return err
	}

	fmt.Println("Launching Substreams GUI...")

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

func loadSubstreamsAuthEnvFile(manifestPath string) {
	var projectPath string
	if manifestPath == "-" {
		// For stdin, use current working directory
		if wd, err := os.Getwd(); err == nil {
			projectPath = wd
		} else {
			projectPath = "."
		}
	} else {
		projectPath = filepath.Dir(manifestPath)
	}
	authFile := filepath.Join(projectPath, ".substreams.env")
	_, err := os.Stat(authFile)
	if err != nil {
		if os.IsNotExist(err) {
			authFile = ".substreams.env"
			_, err := os.Stat(authFile)
			if err != nil {
				if os.IsNotExist(err) {
					return
				} else {
					fmt.Printf("Error reading stats on auth file: %v: %s\n", authFile, err.Error())
					return
				}
			}
		} else {
			fmt.Printf("Error reading stats on auth file: %v: %s\n", authFile, err.Error())
			return
		}
	}

	cnt, err := os.ReadFile(authFile)
	if err != nil {
		fmt.Printf("Error reading auth file: %v: %s\n", authFile, err.Error())
		return
	}

	lines := strings.Split(string(cnt), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "export") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])
			fmt.Printf("Reading %s from %s\n", key, authFile)
			os.Setenv(key, value)
		}
	}
}

func resolveManifestFile(input string) (manifestName string, err error) {
	if input == "" {
		_, err := os.Stat("substreams.yaml")
		if err != nil {
			if os.IsNotExist(err) {
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
