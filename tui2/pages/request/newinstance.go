package request

import (
	"bytes"
	"fmt"
	"log"
	"path/filepath"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/streamingfast/substreams/client"
	"github.com/streamingfast/substreams/manifest"
	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/sink"
	"github.com/streamingfast/substreams/tui2/common"
	"github.com/streamingfast/substreams/tui2/replaylog"
	streamui "github.com/streamingfast/substreams/tui2/stream"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type SetupNewInstanceMsg struct {
	StartStream bool
}

func SetupNewInstanceCmd(startStream bool) tea.Cmd {
	return func() tea.Msg { return SetupNewInstanceMsg{StartStream: startStream} }
}

type NewRequestInstance *Instance

type Instance struct {
	StartStream    bool
	Stream         *streamui.Stream
	MsgDescs       map[string]*manifest.ModuleDescriptor
	ReplayLog      *replaylog.File
	RequestSummary *Summary
	Modules        *pbsubstreams.Modules
	Graph          *manifest.ModuleGraph
}

func NewInstance(sinkerConfig *sink.SinkerConfig, tuiConfig *common.TUIConfig) (out *Instance, err error) {
	// WARN: this is run in a goroutine, so there are risks of races when we mutate
	// this *Config pointer, although it should be fairly low risk.
	// A solution is to clone the Config, and return it back inside the Instance, and apply it back
	// in the Update() cycle.
	readerOptions := []manifest.Option{
		manifest.WithOverrideOutputModule(tuiConfig.OutputModule),
	}

	var params map[string]string
	if tuiConfig.Params != "" {
		params, err = manifest.ParseParams(strings.Split(tuiConfig.Params, "\n"))
		if err != nil {
			return nil, fmt.Errorf("parsing params: %w", err)
		}
		readerOptions = append(readerOptions, manifest.WithParams(params))
	}

	if sinkerConfig.Network != "" {
		readerOptions = append(readerOptions, manifest.WithOverrideNetwork(sinkerConfig.Network))
	}

	manifestReader, err := manifest.NewReader(tuiConfig.ManifestPath, readerOptions...)
	if err != nil {
		return nil, fmt.Errorf("reading package: %w", err)
	}

	pkgBundle, err := manifestReader.Read()
	if err != nil {
		return nil, fmt.Errorf("parsing package at %q: %w", tuiConfig.ManifestPath, err)
	}

	if pkgBundle == nil {
		return nil, fmt.Errorf("no package found")
	}

	if sinkerConfig.Pkg == nil {
		sinkerConfig.Pkg = pkgBundle.Package
	}
	graph := pkgBundle.Graph

	if tuiConfig.OutputModule == "" && graph != nil {
		mods, ok := graph.TopologicalSort()
		if ok {
			tuiConfig.OutputModule = mods[0].Name
		}
	}

	/* PHASE THIS OUT SOME DAY! */
	// Create a custom zap logger that captures the output
	var logBuffer bytes.Buffer
	encoderConfig := zap.NewDevelopmentEncoderConfig()
	encoder := zapcore.NewConsoleEncoder(encoderConfig)
	writer := zapcore.AddSync(&logBuffer)
	core := zapcore.NewCore(encoder, writer, zap.InfoLevel)
	logger := zap.New(core)

	endpoint, err := manifest.ExtractNetworkEndpoint(sinkerConfig.Pkg.Network, sinkerConfig.ClientConfig.Endpoint(), logger)
	if err != nil {
		return nil, fmt.Errorf("extracting network endpoint: %w", err)
	}

	logger.Sync()
	if logBuffer.String() != "" {
		log.Println("Accumulated these logs:", logBuffer.String())
	}
	sinkerConfig.ClientConfig.SetEndpoint(endpoint)

	var startBlock int64 = sinkerConfig.StartBlock
	if graph != nil && tuiConfig.OutputModule != "" {
		if startBlock == 0 {
			sb, err := graph.ModuleInitialBlock(tuiConfig.OutputModule)
			if err != nil {
				return nil, fmt.Errorf("start block: %w", err)
			}
			startBlock = int64(sb)
		}
	}

	var stopBlock uint64 = sinkerConfig.StopBlock
	if stopBlock == 0 {
		// Parse stop block if it was set as string (this might not be needed in new design)
		// stopBlock, err = parseStopBlock(startBlock, stopBlockString, tuiConfig.Cursor != "")
		// if err != nil {
		// 	return nil, fmt.Errorf("invalid stop block: %w", err)
		// }
	}

	// TODO: use the latest `endpoint`, create a new `SubstreamsClientConfig`
	// TODO: if there's an error, we should have a modal dialog box showing the error, instead of
	// showing in the StatusBar, with a "Confirm" or `esc` to close dialog.
	// in big red font, and with the appropriate size.
	ssClient, _, callOpts, headers, err := client.NewSubstreamsClient(sinkerConfig.ClientConfig)
	if err != nil {
		return nil, fmt.Errorf("substreams client setup: %w", err)
	}
	if headers == nil {
		headers = make(map[string]string)
	}

	outputModules := sinkerConfig.DevOutputModules
	if len(outputModules) == 0 && graph != nil && tuiConfig.OutputModule != "" {
		// with no special value, request all 'local' module outputs
		usedModules, err := graph.ModulesDownTo(tuiConfig.OutputModule)
		if err != nil {
			return nil, fmt.Errorf("get used modules: %w", err)
		}
		for _, mod := range usedModules {
			if strings.Contains(mod.Name, ":") {
				continue
			}
			outputModules = append(outputModules, mod.Name)
		}
	} else if len(outputModules) == 1 && outputModules[0] == ".*" {
		outputModules = nil // with special value '.*', request everything, no filtering
	}

	req := &pbsubstreamsrpc.Request{
		StartBlockNum:                       startBlock,
		StartCursor:                         tuiConfig.Cursor,
		FinalBlocksOnly:                     sinkerConfig.FinalBlocksOnly,
		StopBlockNum:                        stopBlock,
		Modules:                             sinkerConfig.Pkg.Modules,
		OutputModule:                        tuiConfig.OutputModule,
		ProductionMode:                      sinkerConfig.Mode == sink.SubstreamsModeProduction,
		DebugInitialStoreSnapshotForModules: sinkerConfig.DevOutputSnapshots,
		LimitProcessedBlocks:                sinkerConfig.LimitProcessedBlocks,
		DevOutputModules:                    outputModules,
	}

	combinedHeaders := headers.Append(tuiConfig.Headers)
	stream := streamui.New(req, ssClient, combinedHeaders, callOpts)

	if err := req.Validate(); err != nil {
		return nil, fmt.Errorf("validate request: %w", err)
	}

	replayLogFilePath := filepath.Join(tuiConfig.HomeDir, "replay.log")
	replayLog := replaylog.New(replaylog.WithPath(replayLogFilePath))
	if tuiConfig.Vcr {
		stream.ReplayBundle, err = replayLog.ReadReplay()
		if err != nil {
			return nil, err
		}
	} else {
		if err := replayLog.OpenForWriting(); err != nil {
			return nil, err
		}
		//defer replayLog.Close()
	}

	debugLogPath := filepath.Join(tuiConfig.HomeDir, "debug.log")
	tea.LogToFile(debugLogPath, "gui:")

	msgDescs, err := manifest.BuildMessageDescriptors(sinkerConfig.Pkg)
	if err != nil {
		return nil, fmt.Errorf("building message descriptors: %w", err)
	}

	requestSummary := &Summary{
		Manifest:        tuiConfig.ManifestPath,
		Endpoint:        sinkerConfig.ClientConfig.Endpoint(),
		ProductionMode:  sinkerConfig.Mode == sink.SubstreamsModeProduction,
		InitialSnapshot: req.DebugInitialStoreSnapshotForModules,
		Docs:            sinkerConfig.Pkg.PackageMeta,
		ModuleDocs:      sinkerConfig.Pkg.ModuleMeta,
		Params:          params,
	}

	substreamRequirements := &Instance{
		Stream:         stream,
		MsgDescs:       msgDescs,
		ReplayLog:      replayLog,
		RequestSummary: requestSummary,
		Modules:        sinkerConfig.Pkg.Modules,
		Graph:          graph,
	}

	return substreamRequirements, nil
}

func parseStopBlock(startBlock int64, stopBlock string, withCursor bool) (uint64, error) {
	isRelative := strings.HasPrefix(stopBlock, "+")
	if isRelative {
		if withCursor {
			return 0, fmt.Errorf("relative stop block is not supported with a cursor")
		}

		if startBlock < 0 {
			return 0, fmt.Errorf("relative end block is supported only with an absolute start block")
		}

		stopBlock = strings.TrimPrefix(stopBlock, "+")
	}

	endBlock, err := strconv.ParseUint(stopBlock, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("end block is invalid: %w", err)
	}

	if isRelative {
		return uint64(startBlock) + endBlock, nil
	}

	return endBlock, nil
}
