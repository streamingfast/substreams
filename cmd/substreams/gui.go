package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/streamingfast/substreams/tui2/pages/request"
	"github.com/streamingfast/substreams/tui2/replaylog"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jhump/protoreflect/desc"
	"github.com/spf13/cobra"
	"github.com/streamingfast/cli"

	"github.com/streamingfast/substreams/client"
	"github.com/streamingfast/substreams/manifest"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/tools"
	"github.com/streamingfast/substreams/tui2"
	streamui "github.com/streamingfast/substreams/tui2/stream"
)

func init() {
	guiCmd.Flags().String("substreams-api-token-envvar", "SUBSTREAMS_API_TOKEN", "name of variable containing Substreams Authentication token")
	guiCmd.Flags().StringP("substreams-endpoint", "e", "mainnet.eth.streamingfast.io:443", "Substreams gRPC endpoint")
	guiCmd.Flags().Bool("insecure", false, "Skip certificate validation on GRPC connection")
	guiCmd.Flags().Bool("plaintext", false, "Establish GRPC connection in plaintext")

	guiCmd.Flags().Int64P("start-block", "s", -1, "Start block to stream from. Defaults to -1, which means the initialBlock of the first module you are streaming")
	guiCmd.Flags().StringP("cursor", "c", "", "Cursor to stream from. Leave blank for no cursor")
	guiCmd.Flags().StringP("stop-block", "t", "0", "Stop block to end stream at, inclusively.")
	guiCmd.Flags().StringSlice("debug-modules-initial-snapshot", nil, "List of 'store' modules from which to print the initial data snapshot (Unavailable in Production Mode")
	guiCmd.Flags().StringSlice("debug-modules-output", nil, "List of extra modules from which to print outputs, deltas and logs (Unavailable in Production Mode)")
	guiCmd.Flags().Bool("production-mode", false, "Enable Production Mode, with high-speed parallel processing")
	guiCmd.Flags().StringSliceP("params", "p", nil, "Set a params for parameterizable modules. Can be specified multiple times. Ex: -p module1=valA -p module2=valX&valY")

	guiCmd.Flags().Bool("replay", false, "Replay saved session into GUI from replay.bin")
	rootCmd.AddCommand(guiCmd)
}

// guiCmd represents the command to run substreams remotely
var guiCmd = &cobra.Command{
	Use:   "gui [<manifest>] <module_name>",
	Short: "Stream module outputs from a given package on a remote endpoint",
	Long: cli.Dedent(`
		Stream module outputs from a given package on a remote endpoint. The manifest is optional as it will try to find a file named 
		'substreams.yaml' in current working directory if nothing entered. You may enter a directory that contains a 'substreams.yaml' 
		file in place of '<manifest_file>'.
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
		if cli.DirectoryExists(args[0]) || cli.FileExists(args[0]) || strings.Contains(args[0], ".") {
			return fmt.Errorf("parameter entered likely a manifest file, don't forget to include a '<module_name>' in your command")
		}

		// At this point, we assume the user invoked `substreams run <module_name>` so we `ResolveManifestFile` using the empty string since no argument has been passed.
		manifestPath, err = tools.ResolveManifestFile("")
		if err != nil {
			return fmt.Errorf("resolving manifest: %w", err)
		}
	}

	manifestReader := manifest.NewReader(manifestPath)
	pkg, err := manifestReader.Read()
	if err != nil {
		return fmt.Errorf("read manifest %q: %w", manifestPath, err)
	}

	if err := applyParams(cmd, pkg); err != nil {
		return err
	}

	productionMode := mustGetBool(cmd, "production-mode")
	debugModulesOutput := mustGetStringSlice(cmd, "debug-modules-output")
	if debugModulesOutput != nil && productionMode {
		return fmt.Errorf("cannot set 'debug-modules-output' in 'production-mode'")
	}

	debugModulesInitialSnapshot := mustGetStringSlice(cmd, "debug-modules-initial-snapshot")

	graph, err := manifest.NewModuleGraph(pkg.Modules.Modules)
	if err != nil {
		return fmt.Errorf("creating module graph: %w", err)
	}

	outputModule := args[0]
	startBlock := mustGetInt64(cmd, "start-block")
	if startBlock == -1 {
		sb, err := graph.ModuleInitialBlock(outputModule)
		if err != nil {
			return fmt.Errorf("getting module start block: %w", err)
		}
		startBlock = int64(sb)
	}

	substreamsClientConfig := client.NewSubstreamsClientConfig(
		mustGetString(cmd, "substreams-endpoint"),
		readAPIToken(cmd, "substreams-api-token-envvar"),
		mustGetBool(cmd, "insecure"),
		mustGetBool(cmd, "plaintext"),
	)

	ssClient, connClose, callOpts, err := client.NewSubstreamsClient(substreamsClientConfig)
	if err != nil {
		return fmt.Errorf("substreams client setup: %w", err)
	}
	defer connClose()

	stopBlock, err := readStopBlockFlag(cmd, startBlock, "stop-block")
	if err != nil {
		return fmt.Errorf("stop block: %w", err)
	}

	req := &pbsubstreams.Request{
		StartBlockNum:                       startBlock,
		StartCursor:                         mustGetString(cmd, "cursor"),
		StopBlockNum:                        stopBlock,
		ForkSteps:                           []pbsubstreams.ForkStep{pbsubstreams.ForkStep_STEP_IRREVERSIBLE},
		Modules:                             pkg.Modules,
		OutputModule:                        outputModule,
		OutputModules:                       []string{outputModule}, //added for backwards compatibility, will be removed
		ProductionMode:                      productionMode,
		DebugInitialStoreSnapshotForModules: debugModulesInitialSnapshot,
	}

	if err := pbsubstreams.ValidateRequest(req, false); err != nil {
		return fmt.Errorf("validate request: %w", err)
	}
	toPrint := debugModulesOutput
	if toPrint == nil {
		toPrint = []string{outputModule}
	}

	msgDescs, err := buildMessageDescriptors(pkg)

	stream := streamui.New(req, ssClient, callOpts)

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

	replayLogFilePath := filepath.Join(homeDir, "replay.log")
	replayLog := replaylog.New(replaylog.WithPath(replayLogFilePath))
	if mustGetBool(cmd, "replay") {
		stream.ReplayBundle, err = replayLog.ReadReplay()
		if err != nil {
			return err
		}
	} else {
		if err := replayLog.OpenForWriting(); err != nil {
			return err
		}
		defer replayLog.Close()
	}

	fmt.Println("Launching Substreams GUI...")

	debugLogPath := filepath.Join(homeDir, "debug.log")
	tea.LogToFile(debugLogPath, "gui:")

	requestSummary := &request.Summary{
		Manifest:        manifestPath,
		Endpoint:        substreamsClientConfig.Endpoint(),
		ProductionMode:  productionMode,
		InitialSnapshot: req.DebugInitialStoreSnapshotForModules,
		Docs:            pkg.PackageMeta,
	}

	ui := tui2.New(stream, msgDescs, replayLog, requestSummary, pkg.Modules)
	prog := tea.NewProgram(ui, tea.WithAltScreen())
	if _, err := prog.Run(); err != nil {
		return fmt.Errorf("gui error: %w", err)
	}

	return nil
}

func buildMessageDescriptors(pkg *pbsubstreams.Package) (out map[string]*desc.MessageDescriptor, err error) {
	fileDescs, err := desc.CreateFileDescriptors(pkg.ProtoFiles)
	if err != nil {
		return nil, fmt.Errorf("couldn't convert, should do this check much earlier: %w", err)
	}

	out = make(map[string]*desc.MessageDescriptor)
	for _, mod := range pkg.Modules.Modules {
		var msgType string
		switch modKind := mod.Kind.(type) {
		case *pbsubstreams.Module_KindStore_:
			msgType = modKind.KindStore.ValueType
		case *pbsubstreams.Module_KindMap_:
			msgType = modKind.KindMap.OutputType
		}
		msgType = strings.TrimPrefix(msgType, "proto:")

		//ui.msgTypes[mod.Name] = msgType
		// replace references to msgTypes by calls to
		// GetFullyQualifiedName() on the msgDesc

		var msgDesc *desc.MessageDescriptor
		for _, file := range fileDescs {
			msgDesc = file.FindMessage(msgType)
			if msgDesc != nil {
				break
			}
		}
		out[mod.Name] = msgDesc
	}
	return
}
