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

type Config struct {
	ManifestPath                string
	Pkg                         *pbsubstreams.Package
	SkipPackageValidation       bool
	Graph                       *manifest.ModuleGraph
	ReadFromModule              bool
	ProdMode                    bool
	DebugModulesOutput          []string
	DebugModulesInitialSnapshot []string
	Endpoint                    string
	StartBlock                  string
	StopBlock                   string
	FinalBlocksOnly             bool
	Headers                     map[string]string
	OutputModule                string
	OverrideNetwork             string
	SubstreamsClientConfig      *client.SubstreamsClientConfig
	HomeDir                     string
	Vcr                         bool
	Cursor                      string
	Params                      string
}

type Instance struct {
	StartStream    bool
	Stream         *streamui.Stream
	MsgDescs       map[string]*manifest.ModuleDescriptor
	ReplayLog      *replaylog.File
	RequestSummary *Summary
	Modules        *pbsubstreams.Modules
	Graph          *manifest.ModuleGraph
}

func (c *Config) NewInstance() (out *Instance, err error) {
	// WARN: this is run in a goroutine, so there are risks of races when we mutate
	// this *Config pointer, although it should be fairly low risk.
	// A solution is to clone the Config, and return it inside the Instance, and apply it back
	// in the Update() cycle.
	readerOptions := []manifest.Option{
		manifest.WithOverrideOutputModule(c.OutputModule),
	}

	var params map[string]string
	if c.Params != "" {
		params, err = manifest.ParseParams(strings.Split(c.Params, "\n"))
		if err != nil {
			return nil, fmt.Errorf("parsing params: %w", err)
		}
		readerOptions = append(readerOptions, manifest.WithParams(params))
	}
	if c.OverrideNetwork != "" {
		readerOptions = append(readerOptions, manifest.WithOverrideNetwork(c.OverrideNetwork))
	}
	if c.SkipPackageValidation {
		readerOptions = append(readerOptions, manifest.SkipPackageValidationReader())
	}

	manifestReader, err := manifest.NewReader(c.ManifestPath, readerOptions...)
	if err != nil {
		return nil, fmt.Errorf("reading package: %w", err)
	}

	pkg, graph, err := manifestReader.Read()
	if err != nil {
		return nil, fmt.Errorf("parsing package at %q: %w", c.ManifestPath, err)
	}

	if c.OutputModule == "" {
		mods, ok := graph.TopologicalSort()
		if ok {
			c.OutputModule = mods[0].Name
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

	endpoint, err := manifest.ExtractNetworkEndpoint(pkg.Network, c.Endpoint, logger)
	if err != nil {
		return nil, fmt.Errorf("extracting network endpoint: %w", err)
	}
	c.Endpoint = endpoint

	logger.Sync()
	if logBuffer.String() != "" {
		log.Println("Accumulated these logs:", logBuffer.String())
	}
	c.SubstreamsClientConfig.SetEndpoint(endpoint)

	c.Pkg = pkg
	c.Graph = graph

	var startBlock int64
	if c.StartBlock != "" {
		// TODO: use the methods for parsing those start blocks..
		startBlock, err = strconv.ParseInt(c.StartBlock, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid start block: %w", err)
		}
	} else {
		sb, err := c.Graph.ModuleInitialBlock(c.OutputModule)
		if err != nil {
			return nil, fmt.Errorf("start block: %w", err)
		}
		startBlock = int64(sb)
	}

	var stopBlock uint64
	if c.StopBlock != "" {
		stopBlock, err = parseStopBlock(startBlock, c.StopBlock, c.Cursor != "")
		if err != nil {
			return nil, fmt.Errorf("invalid stop block: %w", err)
		}
	}

	// TODO: use the latest `endpoint`, create a new `SubstreamsClientConfig`
	// TODO: if there's an error, we should have a modal dialog box showing the error, instead of
	// showing in the StatusBar, with a "Confirm" or `esc` to close dialog.
	// in big red font, and with the appropriate size.
	ssClient, _, callOpts, headers, err := client.NewSubstreamsClient(c.SubstreamsClientConfig)
	if err != nil {
		return nil, fmt.Errorf("substreams client setup: %w", err)
	}
	if headers == nil {
		headers = make(map[string]string)
	}

	req := &pbsubstreamsrpc.Request{
		StartBlockNum:                       startBlock,
		StartCursor:                         c.Cursor,
		FinalBlocksOnly:                     c.FinalBlocksOnly,
		StopBlockNum:                        stopBlock,
		Modules:                             c.Pkg.Modules,
		OutputModule:                        c.OutputModule,
		ProductionMode:                      c.ProdMode,
		DebugInitialStoreSnapshotForModules: c.DebugModulesInitialSnapshot,
	}

	c.Headers = headers.Append(c.Headers)
	stream := streamui.New(req, ssClient, c.Headers, callOpts)

	if err := req.Validate(); err != nil {
		return nil, fmt.Errorf("validate request: %w", err)
	}

	replayLogFilePath := filepath.Join(c.HomeDir, "replay.log")
	replayLog := replaylog.New(replaylog.WithPath(replayLogFilePath))
	if c.Vcr {
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

	debugLogPath := filepath.Join(c.HomeDir, "debug.log")
	tea.LogToFile(debugLogPath, "gui:")

	msgDescs, err := manifest.BuildMessageDescriptors(c.Pkg)
	if err != nil {
		return nil, fmt.Errorf("building message descriptors: %w", err)
	}

	requestSummary := &Summary{
		Manifest:        c.ManifestPath,
		Endpoint:        c.SubstreamsClientConfig.Endpoint(),
		ProductionMode:  c.ProdMode,
		InitialSnapshot: req.DebugInitialStoreSnapshotForModules,
		Docs:            c.Pkg.PackageMeta,
		ModuleDocs:      c.Pkg.ModuleMeta,
		Params:          params,
	}

	substreamRequirements := &Instance{
		Stream:         stream,
		MsgDescs:       msgDescs,
		ReplayLog:      replayLog,
		RequestSummary: requestSummary,
		Modules:        c.Pkg.Modules,
		Graph:          c.Graph,
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
