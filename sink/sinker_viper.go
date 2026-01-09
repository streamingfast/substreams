package sink

import (
	"fmt"
	"time"

	"github.com/bobg/go-generics/v2/slices"
	"github.com/cenkalti/backoff/v4"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/streamingfast/cli/sflags"
	"github.com/streamingfast/logging"
	"github.com/streamingfast/substreams/client"
	"github.com/streamingfast/substreams/manifest"
	"go.uber.org/zap"
)

const (
	FlagEndpoint        = "endpoint"
	ShortFlagEndoint    = "e"
	FlagStartBlock      = "start-block"
	ShortFlagStartBlock = "s"
	FlagStopBlock       = "stop-block"
	ShortFlagStopBlock  = "t"
	FlagCursor          = "cursor"
	ShortFlagCursor     = "c"

	FlagFinalBlocksOnly = "final-blocks-only"
	FlagAPIKeyEnvvar    = "api-key-envvar"
	FlagAPITokenEnvvar  = "api-token-envvar"

	FlagNetwork              = "network"
	FlagParams               = "params"
	FlagInsecure             = "insecure"
	FlagPlaintext            = "plaintext"
	FlagForceProtocolVersion = "force-protocol-version"

	FlagUndoBufferSize     = "undo-buffer-size"
	FlagLiveBlockTimeDelta = "live-block-time-delta"
	FlagDevelopmentMode    = "development-mode"
	FlagMaxRetries         = "max-retries"

	FlagSkipPackageValidation        = "skip-package-validation"
	FlagPartialBlocks                = "partial-blocks"
	FlagExtraHeaders                 = "header"
	FlagNoopMode                     = "noop-mode"
	FlagProtoPath                    = "proto-path"
	FlagProtoDescriptorSet           = "proto-descriptor-set"
	FlagPrometheusAddr               = "prometheus-addr"
	FlagSkipCheckModuleBinariesExist = "skip-check-module-binaries-exist"
)

type FlagInclusionExclusion interface {
	IsExplicitlyIncluded(flag string) bool
	IsExplicitlyExcluded(flag string) bool
}

// Deprecated: Use FlagInclusionExclusion instead
type FlagIgnored = FlagInclusionExclusion

// FlagExcludeDefault can be used to exclude one or more of the default flags from being added to a flag set
// when using [AddFlagsToSet].
func FlagExcludeDefault(in ...string) FlagInclusionExclusion {
	return flagsExclusion(in)
}

// FlagIncludeOptional can be used to include one or more optional flags to be added to a flag set
// when using [AddFlagsToSet].
func FlagIncludeOptional(in ...string) FlagInclusionExclusion {
	return flagsInclusion(in)
}

// Deprecated: use FlagExclude instead
var FlagIgnore = FlagExcludeDefault

type flagsExclusion []string

func (i flagsExclusion) IsExplicitlyIncluded(flag string) bool {
	return false
}

func (i flagsExclusion) IsExplicitlyExcluded(flag string) bool {
	return slices.Contains(i, flag)
}

type flagsInclusion []string

func (i flagsInclusion) IsExplicitlyIncluded(flag string) bool {
	return slices.Contains(i, flag)
}

func (i flagsInclusion) IsExplicitlyExcluded(flag string) bool {
	return false
}

// AddFlagsToSet can be used to import standard flags a controls which flags to ignore (if default
// is to include it) or include (if default is to ignore it).
//
//	needed for sink to configure itself. By using
//
// this method to define your flag and using `cli.ConfigureViper` (import "github.com/streamingfast/cli")
// in your main application command, `NewFromViper` is usable to easily create a `sink.Sinker` instance.
//
// # Default Inclusion
//
// Added by default: [FlagEndpoint], [FlagStartBlock], [FlagStopBlock], [FlagNetwork], [FlagParams],
// [FlagInsecure], [FlagPlaintext], [FlagUndoBufferSize], [FlagLiveBlockTimeDelta], [FlagDevelopmentMode],
// [FlagFinalBlocksOnly], [FlagMaxRetries], [FlagSkipPackageValidation], [FlagExtraHeaders],
// [FlagAPIKeyEnvvar], [FlagAPITokenEnvvar], [FlagProtoPath], [FlagProtoDescriptorSet], [FlagPrometheusAddr]
//
// To avoid adding a default included flag, use [FlagExcludeDefault(<flag>, ...)].
//
// # Possible Inclusion
//
// Not added by default but can be explicitly included: [FlagCursor].
//
// You add those flags by using [FlagIncludeOptional(<flag>, ...)].
//
// # Reference Table
//
//	[FlagEndpoint]: `--endpoint` (-e) (defaults `""`)
//	[FlagStartBlock]: `--start-block` (-s) (defaults `""`)
//	[FlagStopBlock]: `--stop-block` (-t) (defaults `""`)
//	[FlagCursor]: `--cursor` (-c) (defaults `""`)
//	[FlagNetwork]: `--network` (defaults `""`)
//	[FlagParams]: `--params` (-p) (defaults `[]`)
//	[FlagInsecure]: `--insecure` (defaults `false`)
//	[FlagPlaintext]: `--plaintext` (defaults `false`)
//	[FlagUndoBufferSize]: `--undo-buffer-size` (defaults `12`)
//	[FlagLiveBlockTimeDelta]: `--live-block-time-delta` (defaults `300*time.Second`)
//	[FlagDevelopmentMode]: `--development-mode` (defaults `false`)
//	[FlagNoopMode]: `--noop-mode` (defaults `false`)
//	[FlagFinalBlocksOnly]: `--final-blocks-only` (defaults `false`)
//	[FlagMaxRetries]: `--max-retries` (defaults `3`)
//	[FlagSkipPackageValidation]: `--skip-package-validation` (defaults `false`)
//	[FlagExtraHeaders]: `--header` (-H) (defaults `[]`)
//	[FlagAPIKeyEnvvar]: `--api-key-envvar` (default `SUBSTREAMS_API_KEY`)
//	[FlagAPITokenEnvvar]: `--api-token-envvar` (default `SUBSTREAMS_API_TOKEN`)
//	[FlagProtoPath]: `--proto-path` (defaults `""`)
//	[FlagProtoDescriptorSet]: `--proto-descriptor-set` (defaults `""`)
//	[FlagPrometheusAddr]: `--prometheus-addr` (defaults `""`)
func AddFlagsToSet(flags *pflag.FlagSet, ignore ...FlagInclusionExclusion) {
	defaultFlagIncluded := func(x string) bool {
		return every(ignore, func(e FlagInclusionExclusion) bool { return !e.IsExplicitlyExcluded(x) })
	}
	optionalFlagIncluded := func(x string) bool {
		return some(ignore, func(e FlagInclusionExclusion) bool { return e.IsExplicitlyIncluded(x) })
	}

	if defaultFlagIncluded(FlagEndpoint) {
		flags.StringP(FlagEndpoint, ShortFlagEndoint, "", "Substreams gRPC endpoint (supports http:// or https:// prefix). If empty, will be replaced by the SUBSTREAMS_ENDPOINT_{network_name} environment variable, where `network_name` is determined from the substreams manifest. Defaults to SSL unless prefixed with `http://` or if `--plaintext` flag is used.")
	}

	if defaultFlagIncluded(FlagStartBlock) {
		flags.StringP(FlagStartBlock, ShortFlagStartBlock, "", "Start block to stream from. If empty, will be replaced by initialBlock of the first module you are streaming. If negative, will be resolved by the server relative to the chain head")
	}

	if defaultFlagIncluded(FlagStopBlock) {
		flags.StringP(FlagStopBlock, ShortFlagStopBlock, "0", "Stop block to end stream at, exclusively. If the start-block is positive, a '+' prefix can indicate 'relative to start-block'")
	}

	if optionalFlagIncluded(FlagCursor) {
		flags.StringP(FlagCursor, ShortFlagCursor, "", "Cursor to stream from. Leave blank for no cursor")
	}

	if optionalFlagIncluded(FlagSkipCheckModuleBinariesExist) {
		flags.Bool(FlagSkipCheckModuleBinariesExist, true, "Skip validation that the module binaries exist when loading from YAML manifest (ex: WASM already built)")
	}

	if optionalFlagIncluded(FlagPartialBlocks) {
		flags.Bool(FlagPartialBlocks, false, "Stream partial blocks in the stream")
	}

	if defaultFlagIncluded(FlagNetwork) {
		flags.String(FlagNetwork, "", "Specify the network to use for params and initialBlocks, overriding the 'network' field in the substreams package")
	}

	if defaultFlagIncluded(FlagParams) {
		flags.StringArrayP(FlagParams, "p", nil, "Set a params for parameterizable modules. Can be specified multiple times. Ex: -p module1=valA -p module2=valX&valY")
	}

	if defaultFlagIncluded(FlagInsecure) {
		flags.Bool(FlagInsecure, false, "Skip certificate validation on GRPC connection")
	}

	if defaultFlagIncluded(FlagPlaintext) {
		flags.Bool(FlagPlaintext, false, "Use plaintext connection as default for endpoints without an http:// or https:// prefix")
	}

	if defaultFlagIncluded(FlagForceProtocolVersion) {
		flags.Int(FlagForceProtocolVersion, 0, "Force the use of a specific protocol version (0=unset: v3 with fallback, 2=v2, 3=v3)")
	}

	if defaultFlagIncluded(FlagUndoBufferSize) {
		flags.Int(FlagUndoBufferSize, 0, "Number of blocks to keep buffered to handle fork reorganizations")
	}

	if defaultFlagIncluded(FlagLiveBlockTimeDelta) {
		flags.Duration(FlagLiveBlockTimeDelta, 0, "Consider chain live if block time is within this number of seconds of current time. If disabled, liveness is based on the cursor being 'finalized' or not")
	}

	if defaultFlagIncluded(FlagDevelopmentMode) {
		flags.Bool(FlagDevelopmentMode, false, "Enable development mode, use it for testing purpose only, should not be used for production workload")
	}

	if defaultFlagIncluded(FlagNoopMode) {
		flags.Bool(FlagNoopMode, false, "Sends the request to the server with 'noop-mode', which will not send actual data, only populate the cache")
	}

	if defaultFlagIncluded(FlagFinalBlocksOnly) {
		flags.Bool(FlagFinalBlocksOnly, false, "Only process blocks that have pass finality, to prevent any reorg and undo signal by staying further away from the chain HEAD")
	}

	if defaultFlagIncluded(FlagMaxRetries) {
		flags.Int(FlagMaxRetries, 3, "Maximum number of retries for substreams calls (0 disables retries, -1 for infinite retries) -- Counter is reset when we receive actual data")
	}

	if defaultFlagIncluded(FlagSkipPackageValidation) {
		flags.Bool(FlagSkipPackageValidation, false, "Do not perform any validation when reading substreams package")
	}

	if defaultFlagIncluded(FlagExtraHeaders) {
		flags.StringSliceP(FlagExtraHeaders, "H", nil, "Additional headers to be sent in the substreams request")
	}

	if defaultFlagIncluded(FlagAPIKeyEnvvar) {
		flags.String(FlagAPIKeyEnvvar, "SUBSTREAMS_API_KEY", "Name of variable containing Substreams Api Key")
	}

	if defaultFlagIncluded(FlagAPITokenEnvvar) {
		flags.String(FlagAPITokenEnvvar, "SUBSTREAMS_API_TOKEN", "name of variable containing Substreams Authentication token")
	}

	if defaultFlagIncluded(FlagProtoPath) {
		flags.String(FlagProtoPath, "", "Path to proto files")
	}

	if defaultFlagIncluded(FlagProtoDescriptorSet) {
		flags.String(FlagProtoDescriptorSet, "", "Path to proto descriptor set file")
	}

	if defaultFlagIncluded(FlagPrometheusAddr) {
		flags.String(FlagPrometheusAddr, "localhost:9102", "Address to bind prometheus metrics server")
	}

}

// NewFromViper constructs a new Sinker instance from a fixed set of "known" flags.
// This function creates a SinkerConfig from the Viper configuration and then uses
// it to create a new Sinker instance.
//
// If you want to extract the sink output module's name directly from the Substreams
// package, if supported by your sink, instead of an actual name for paramater
// `outputModuleNameArg`, use `sink.InferOutputModuleFromPackage`.
//
// The `expectedOutputModuleType` should be the fully qualified expected Protobuf
// package.
//
// The `manifestPath` can be left empty in which case we this method is going to look
// in the current directory for a `substreams.yaml` file. If the `manifestPath` is
// non-empty and points to a directory, we will look for a `substreams.yaml` file in that
// directory.
func NewFromViper(
	cmd *cobra.Command,
	expectedOutputModuleType string,
	manifestPath string,
	outputModuleName string,
	userAgent string,
	zlog *zap.Logger,
	tracer logging.Tracer,
) (*Sinker, error) {
	config, err := ConfigFromViper(cmd, expectedOutputModuleType, manifestPath, outputModuleName, userAgent, zlog, tracer)
	if err != nil {
		return nil, fmt.Errorf("creating sinker config from viper: %w", err)
	}

	return NewFromConfig(config)
}

// ConfigFromViper creates a [SinkerConfig] from the provided Viper configuration.
// This function extracts all necessary configuration from the command flags and
// creates a complete [SinkerConfig] that can be used to create a Sinker instance.
// The endpoint is read from the [FlagEndpoint] flag, and the block range is computed
// from the [FlagStartBlock] and [FlagStopBlock] flags.
//
// If you want to dynamically validate the output module type yourself, for argument
// `expectedOutputModuleType`, you can pass [IgnoreOutputModuleType] or the empty
// string.
func ConfigFromViper(
	cmd *cobra.Command,
	expectedOutputModuleType string,
	manifestPath string,
	outputModuleName string,
	userAgent string,
	zlog *zap.Logger,
	tracer logging.Tracer,
) (*SinkerConfig, error) {
	params, network, undoBufferSize, liveBlockTimeDelta, isDevelopmentMode, maxRetries, finalBlocksOnly, partialBlocks, skipPackageValidation, isNoopMode, extraHeaders, prometheusAddr, skipCheckModuleBinariesExist := getViperFlags(cmd)

	// Parse start and stop blocks using utility functions
	startBlockFlag := sflags.MustGetString(cmd, FlagStartBlock)
	startBlock, startBlockIsEmpty, err := parseStartBlockFlag(startBlockFlag)
	if err != nil {
		return nil, fmt.Errorf("reading start block flag: %w", err)
	}

	var readerOptions []manifest.Option
	if protoPath, err := sflags.GetString(cmd, FlagProtoPath); err == nil {
		if protoPath != "" {
			readerOptions = append(readerOptions, manifest.WithProtoPath(protoPath))
		}
	}

	if protoDescriptorSet, err := sflags.GetString(cmd, FlagProtoDescriptorSet); err == nil {
		if protoDescriptorSet != "" {
			readerOptions = append(readerOptions, manifest.WithProtoDescriptorSet(protoDescriptorSet))
		}
	}

	if outputModuleName != "" {
		readerOptions = append(readerOptions, manifest.WithOverrideOutputModule(outputModuleName))
	}

	if skipCheckModuleBinariesExist {
		readerOptions = append(readerOptions, manifest.SkipSourceCodeReader())
	}

	pkg, module, outputModuleHash, err := ReadManifestAndModule(
		manifestPath,
		network,
		params,
		outputModuleName,
		expectedOutputModuleType,
		skipPackageValidation,
		readerOptions,
		zlog,
	)
	if err != nil {
		return nil, fmt.Errorf("reading manifest: %w", err)
	}

	// Get endpoint from flags
	endpoint, err := manifest.ExtractNetworkEndpoint(pkg.Network, sflags.MustGetString(cmd, FlagEndpoint), zlog)
	if err != nil {
		return nil, fmt.Errorf("extracting endpoint: %w", err)
	}

	// Resolve start block if empty (use module's initial block)
	if startBlockIsEmpty {
		startBlock = int64(module.InitialBlock)
	}

	stopBlockFlag := sflags.MustGetString(cmd, FlagStopBlock)
	stopBlock, err := parseStopBlockFlag(stopBlockFlag, startBlock, cmd.Flags().Lookup(FlagStopBlock).Changed)
	if err != nil {
		return nil, fmt.Errorf("reading stop block flag: %w", err)
	}

	zlog.Info("sinker from CLI",
		zap.String("endpoint", endpoint),
		zap.String("manifest_path", manifestPath),
		zap.Strings("params", params),
		zap.String("network", network),
		zap.String("output_module_name", outputModuleName),
		zap.Stringer("expected_module_type", expectedModuleType(expectedOutputModuleType)),
		zap.Int64("start_block", startBlock),
		zap.Uint64("stop_block", stopBlock),
		zap.Bool("start_block_empty", startBlockIsEmpty),
		zap.Bool("development_mode", isDevelopmentMode),
		zap.Bool("noop_mode", isNoopMode),
		zap.Int("max_retries", maxRetries),
		zap.Bool("final_blocks_only", finalBlocksOnly),
		zap.Bool("skip_package_validation", skipPackageValidation),
		zap.Duration("live_block_time_delta", liveBlockTimeDelta),
		zap.Int("undo_buffer_size", undoBufferSize),
		zap.Strings("extra_headers", extraHeaders),
	)

	if finalBlocksOnly {
		zlog.Debug("override undo buffer size to 0 since final blocks only is requested")
		undoBufferSize = 0
	}

	auth := newAuthenticator(sflags.MustGetString(cmd, FlagAPIKeyEnvvar), sflags.MustGetString(cmd, FlagAPITokenEnvvar))
	authToken, authType := auth.GetTokenAndType()

	protocolVersionFlag := sflags.MustGetInt(cmd, FlagForceProtocolVersion)
	forceProtocolVersion, err := client.ParseProtocolVersion(protocolVersionFlag)
	if err != nil {
		return nil, fmt.Errorf("invalid protocol version: %w", err)
	}

	clientConfig := client.NewSubstreamsClientConfig(client.SubstreamsClientConfigOptions{
		Endpoint:             endpoint,
		AuthToken:            authToken,
		AuthType:             authType,
		Insecure:             sflags.MustGetBool(cmd, FlagInsecure),
		PlainText:            sflags.MustGetBool(cmd, FlagPlaintext),
		Agent:                userAgent,
		ForceProtocolVersion: forceProtocolVersion,
	})

	mode := SubstreamsModeProduction
	if isDevelopmentMode {
		mode = SubstreamsModeDevelopment
	}

	// Create backoff configuration
	bo := backoff.NewExponentialBackOff()
	bo.MaxElapsedTime = 0

	// Create liveness checker if configured
	var livenessChecker LivenessChecker
	if liveBlockTimeDelta > 0 {
		livenessChecker = NewDeltaLivenessChecker(liveBlockTimeDelta)
	} else {
		livenessChecker = NewCursorBasedLivenessChecker()
	}

	config := &SinkerConfig{
		Pkg:                   pkg,
		OutputModule:          module,
		OutputModuleHash:      outputModuleHash,
		ClientConfig:          clientConfig,
		Mode:                  mode,
		NoopMode:              isNoopMode,
		StartBlock:            startBlock,
		StopBlock:             stopBlock,
		UndoBufferSize:        undoBufferSize,
		FinalBlocksOnly:       finalBlocksOnly,
		PartialBlocks:         partialBlocks,
		MaxRetries:            maxRetries,
		BackOff:               bo,
		LiveBlockTimeDelta:    liveBlockTimeDelta,
		LivenessChecker:       livenessChecker,
		ExtraHeaders:          extraHeaders,
		PrometheusAddr:        prometheusAddr,
		Params:                params,
		Network:               network,
		SkipPackageValidation: skipPackageValidation,
		Logger:                zlog,
		Tracer:                tracer,
	}

	return config, nil
}

// CloneWithNewModule creates a new SinkerConfig based on the current one but
// replacing the package, output module and output module hash with the ones
// read from the manifest at `manifestPath`, using `outputModuleName` as the
// output module to extract from the package.
//
// The rest of the configuration remains the same and you can modify it
// further if needed on the returned SinkerConfig.
func (config *SinkerConfig) CloneWithNewModule(
	expectedOutputModuleType string,
	manifestPath string,
	outputModuleName string,
) (*SinkerConfig, error) {
	newPkg, newModule, newOutputModuleHash, err := ReadManifestAndModule(
		manifestPath,
		config.Network,
		config.Params,
		outputModuleName,
		expectedOutputModuleType,
		config.SkipPackageValidation,
		[]manifest.Option{
			manifest.WithOverrideOutputModule(outputModuleName),
		},
		config.Logger,
	)
	if err != nil {
		return nil, fmt.Errorf("reading manifest in CloneWithModule: %w", err)
	}

	newConfig := *config
	newConfig.Pkg = newPkg
	newConfig.OutputModule = newModule
	newConfig.OutputModuleHash = newOutputModuleHash

	return &newConfig, nil
}

func getViperFlags(cmd *cobra.Command) (
	params []string,
	network string,
	undoBufferSize int,
	liveBlockTimeDelta time.Duration,
	isDevelopmentMode bool,
	maxRetries int,
	finalBlocksOnly bool,
	partialBlocks bool,
	skipPackageValidation bool,
	isNoopMode bool,
	extraHeaders []string,
	prometheusAddr string,
	skipCheckModuleBinariesExist bool,
) {
	if sflags.FlagDefined(cmd, FlagParams) {
		params = sflags.MustGetStringArray(cmd, FlagParams)
	}

	if sflags.FlagDefined(cmd, FlagNetwork) {
		network = sflags.MustGetString(cmd, FlagNetwork)
	}

	if sflags.FlagDefined(cmd, FlagUndoBufferSize) {
		undoBufferSize = sflags.MustGetInt(cmd, FlagUndoBufferSize)
	}

	if sflags.FlagDefined(cmd, FlagLiveBlockTimeDelta) {
		liveBlockTimeDelta = sflags.MustGetDuration(cmd, FlagLiveBlockTimeDelta)
	}

	if sflags.FlagDefined(cmd, FlagDevelopmentMode) {
		isDevelopmentMode = sflags.MustGetBool(cmd, FlagDevelopmentMode)
	}

	if sflags.FlagDefined(cmd, FlagNoopMode) {
		isNoopMode = sflags.MustGetBool(cmd, FlagNoopMode)
	}

	if sflags.FlagDefined(cmd, FlagMaxRetries) {
		maxRetries = sflags.MustGetInt(cmd, FlagMaxRetries)
	}

	if sflags.FlagDefined(cmd, FlagFinalBlocksOnly) {
		finalBlocksOnly = sflags.MustGetBool(cmd, FlagFinalBlocksOnly)
	}

	if sflags.FlagDefined(cmd, FlagSkipPackageValidation) {
		skipPackageValidation = sflags.MustGetBool(cmd, FlagSkipPackageValidation)
	}

	if sflags.FlagDefined(cmd, FlagPartialBlocks) {
		partialBlocks = sflags.MustGetBool(cmd, FlagPartialBlocks)
	}

	if sflags.FlagDefined(cmd, FlagExtraHeaders) {
		extraHeaders = sflags.MustGetStringSlice(cmd, FlagExtraHeaders)
	}

	if sflags.FlagDefined(cmd, FlagPrometheusAddr) {
		prometheusAddr = sflags.MustGetString(cmd, FlagPrometheusAddr)
	}

	if sflags.FlagDefined(cmd, FlagSkipCheckModuleBinariesExist) {
		skipCheckModuleBinariesExist = sflags.MustGetBool(cmd, FlagSkipCheckModuleBinariesExist)
	}

	return
}

func every[E any](s []E, test func(e E) bool) bool {
	for _, element := range s {
		if !test(element) {
			return false
		}
	}

	return true
}

func some[E any](s []E, test func(e E) bool) bool {
	for _, element := range s {
		if test(element) {
			return true
		}
	}

	return false
}
