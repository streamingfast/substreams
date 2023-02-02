package main

import (
	"fmt"
	"strings"

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
	guiCmd.Flags().StringP("substreams-endpoint", "e", "mainnet.eth.streamingfast.io:443", "Substreams gRPC endpoint")
	guiCmd.Flags().String("substreams-api-token-envvar", "SUBSTREAMS_API_TOKEN", "name of variable containing Substreams Authentication token")
	guiCmd.Flags().Int64P("start-block", "s", -1, "Start block to stream from. Defaults to -1, which means the initialBlock of the first module you are streaming")
	guiCmd.Flags().StringP("cursor", "c", "", "Cursor to stream from. Leave blank for no cursor")
	guiCmd.Flags().StringP("stop-block", "t", "0", "Stop block to end stream at, inclusively.")
	guiCmd.Flags().BoolP("insecure", "k", false, "Skip certificate validation on GRPC connection")
	guiCmd.Flags().BoolP("plaintext", "p", false, "Establish GRPC connection in plaintext")
	guiCmd.Flags().StringSlice("debug-modules-initial-snapshot", nil, "List of 'store' modules from which to print the initial data snapshot (Unavailable in Production Mode")
	guiCmd.Flags().StringSlice("debug-modules-output", nil, "List of extra modules from which to print outputs, deltas and logs (Unavailable in Production Mode)")
	guiCmd.Flags().Bool("production-mode", false, "Enable Production Mode, with high-speed parallel processing")
	rootCmd.AddCommand(guiCmd)
}

// guiCmd represents the command to run substreams remotely
var guiCmd = &cobra.Command{
	Use:   "gui [<manifest>] <module_name>",
	Short: "Stream module outputs from a given package on a remote endpoint",
	Long: cli.Dedent(`
		Stream module outputs from a given package on a remote endpoint. The manifest is optional as it will try to find one a file named 
		'substreams.yaml' in current working directory if nothing entered. You may enter a directory that contains a 'substreams.yaml' 
		file in place of '<manifest_file>'.
	`),
	RunE:         runGui,
	Args:         cobra.RangeArgs(1, 2),
	SilenceUsage: true,
}

func runGui(cmd *cobra.Command, args []string) error {
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

	tea.LogToFile("debug.log", "gui")
	_, err = tea.NewProgram(tui2.New(stream, msgDescs), tea.WithAltScreen()).Run()
	if err != nil {
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