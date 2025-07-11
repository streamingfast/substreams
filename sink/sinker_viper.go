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

	FlagNetwork   = "network"
	FlagParams    = "params"
	FlagInsecure  = "insecure"
	FlagPlaintext = "plaintext"

	FlagUndoBufferSize     = "undo-buffer-size"
	FlagLiveBlockTimeDelta = "live-block-time-delta"
	FlagDevelopmentMode    = "development-mode"
	FlagInfiniteRetry      = "infinite-retry"

	FlagSkipPackageValidation = "skip-package-validation"
	FlagExtraHeaders          = "header"
	FlagNoopMode              = "noop-mode"
	FlagProtoPath             = "proto-path"
	FlagProtoDescriptorSet    = "proto-descriptor-set"
)

func FlagIgnore(in ...string) FlagIgnored {
	return flagIgnoredList(in)
}

type FlagIgnored interface {
	IsIgnored(flag string) bool
}

type flagIgnoredList []string

func (i flagIgnoredList) IsIgnored(flag string) bool {
	return slices.Contains(i, flag)
}

// AddFlagsToSet can be used to import standard flags needed for sink to configure itself. By using
// this method to define your flag and using `cli.ConfigureViper` (import "github.com/streamingfast/cli")
// in your main application command, `NewFromViper` is usable to easily create a `sink.Sinker` instance.
//
// Defines
//
//	Flag `--endpoint` (-e) (defaults `""`)
//	Flag `--start-block` (-s) (defaults `""`)
//	Flag `--stop-block` (-t) (defaults `""`)
//	Flag `--cursor` (-c) (defaults `""`)
//	Flag `--network` (defaults `""`)
//	Flag `--params` (-p) (defaults `[]`)
//	Flag `--insecure` (defaults `false`)
//	Flag `--plaintext` (defaults `false`)
//	Flag `--undo-buffer-size` (defaults `12`)
//	Flag `--live-block-time-delta` (defaults `300*time.Second`)
//	Flag `--development-mode` (defaults `false`)
//	Flag `--noop-mode` (defaults `false`)
//	Flag `--final-blocks-only` (defaults `false`)
//	Flag `--infinite-retry` (defaults `false`)
//	Flag `--skip-package-validation` (defaults `false`)
//	Flag `--header` (-H) (defaults `[]`)
//	Flag `--api-key-envvar` (default `SUBSTREAMS_API_KEY`)
//	Flag `--api-token-envvar` (default `SUBSTREAMS_API_TOKEN`)
//	Flag `--proto-path` (defaults `""`)
//	Flag `--proto-descriptor-set` (defaults `""`)
//
// The `ignore` field can be used to multiple times to avoid adding the specified
// `flags` to the the set. This can be used for example to avoid adding `--final-blocks-only`
// when the sink is always final only.
//
//	AddFlagsToSet(flags, sink.FlagIgnore(sink.FlagFinalBlocksOnly))
func AddFlagsToSet(flags *pflag.FlagSet, ignore ...FlagIgnored) {
	flagIncluded := func(x string) bool { return every(ignore, func(e FlagIgnored) bool { return !e.IsIgnored(x) }) }

	if flagIncluded(FlagEndpoint) {
		flags.StringP(FlagEndpoint, ShortFlagEndoint, "", "Substreams gRPC endpoint. If empty, will be replaced by the SUBSTREAMS_ENDPOINT_{network_name} environment variable, where `network_name` is determined from the substreams manifest. Some network names have default endpoints.")
	}

	if flagIncluded(FlagStartBlock) {
		flags.StringP(FlagStartBlock, ShortFlagStartBlock, "", "Start block to stream from. If empty, will be replaced by initialBlock of the first module you are streaming. If negative, will be resolved by the server relative to the chain head")
	}

	if flagIncluded(FlagStopBlock) {
		flags.StringP(FlagStopBlock, ShortFlagStopBlock, "0", "Stop block to end stream at, exclusively. If the start-block is positive, a '+' prefix can indicate 'relative to start-block'")
	}

	if flagIncluded(FlagCursor) {
		flags.StringP(FlagCursor, ShortFlagCursor, "", "Cursor to stream from. Leave blank for no cursor")
	}

	if flagIncluded(FlagNetwork) {
		flags.String(FlagNetwork, "", "Specify the network to use for params and initialBlocks, overriding the 'network' field in the substreams package")
	}

	if flagIncluded(FlagParams) {
		flags.StringArrayP(FlagParams, "p", nil, "Set a params for parameterizable modules. Can be specified multiple times. Ex: -p module1=valA -p module2=valX&valY")
	}

	if flagIncluded(FlagInsecure) {
		flags.Bool(FlagInsecure, false, "Skip certificate validation on GRPC connection")
	}

	if flagIncluded(FlagPlaintext) {
		flags.Bool(FlagPlaintext, false, "Establish GRPC connection in plaintext")
	}

	if flagIncluded(FlagUndoBufferSize) {
		flags.Int(FlagUndoBufferSize, 0, "Number of blocks to keep buffered to handle fork reorganizations")
	}

	if flagIncluded(FlagLiveBlockTimeDelta) {
		flags.Duration(FlagLiveBlockTimeDelta, 300*time.Second, "Consider chain live if block time is within this number of seconds of current time")
	}

	if flagIncluded(FlagDevelopmentMode) {
		flags.Bool(FlagDevelopmentMode, false, "Enable development mode, use it for testing purpose only, should not be used for production workload")
	}

	if flagIncluded(FlagNoopMode) {
		flags.Bool(FlagNoopMode, false, "Sends the request to the server with 'noop-mode', which will not send actual data, only populate the cache")
	}

	if flagIncluded(FlagFinalBlocksOnly) {
		flags.Bool(FlagFinalBlocksOnly, false, "Only process blocks that have pass finality, to prevent any reorg and undo signal by staying further away from the chain HEAD")
	}

	if flagIncluded(FlagInfiniteRetry) {
		flags.Bool(FlagInfiniteRetry, false, "Default behavior is to retry 15 times spanning approximatively 5m before exiting with an error, activating this flag will retry forever")
	}

	if flagIncluded(FlagSkipPackageValidation) {
		flags.Bool(FlagSkipPackageValidation, false, "Do not perform any validation when reading substreams package")
	}

	if flagIncluded(FlagExtraHeaders) {
		flags.StringArrayP(FlagExtraHeaders, "H", nil, "Additional headers to be sent in the substreams request")
	}

	if flagIncluded(FlagAPIKeyEnvvar) {
		flags.String(FlagAPIKeyEnvvar, "SUBSTREAMS_API_KEY", "Name of variable containing Substreams Api Key")
	}

	if flagIncluded(FlagAPITokenEnvvar) {
		flags.String(FlagAPITokenEnvvar, "SUBSTREAMS_API_TOKEN", "name of variable containing Substreams Authentication token")
	}

	if flagIncluded(FlagProtoPath) {
		flags.String(FlagProtoPath, "", "Path to proto files")
	}

	if flagIncluded(FlagProtoDescriptorSet) {
		flags.String(FlagProtoDescriptorSet, "", "Path to proto descriptor set file")
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

	return New(config), nil
}

// ConfigFromViper creates a SinkerConfig from the provided Viper configuration.
// This function extracts all necessary configuration from the command flags and
// creates a complete SinkerConfig that can be used to create a Sinker instance.
// The endpoint is read from the FlagEndpoint flag, and the block range is computed
// from the FlagStartBlock and FlagStopBlock flags.
func ConfigFromViper(
	cmd *cobra.Command,
	expectedOutputModuleType string,
	manifestPath string,
	outputModuleName string,
	userAgent string,
	zlog *zap.Logger,
	tracer logging.Tracer,
) (*SinkerConfig, error) {
	params, network, undoBufferSize, liveBlockTimeDelta, isDevelopmentMode, infiniteRetry, finalBlocksOnly, skipPackageValidation, isNoopMode, extraHeaders := getViperFlags(cmd)

	// Get endpoint from flags
	endpoint := sflags.MustGetString(cmd, FlagEndpoint)

	// Parse start and stop blocks using utility functions
	startBlock, startBlockIsEmpty, err := readStartBlockFlag(cmd, FlagStartBlock)
	if err != nil {
		return nil, fmt.Errorf("reading start block flag: %w", err)
	}

	var stopBlock uint64
	stopBlock, err = readStopBlockFlag(cmd, startBlock, FlagStopBlock)
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
		zap.Bool("infinite_retry", infiniteRetry),
		zap.Bool("final_blocks_only", finalBlocksOnly),
		zap.Bool("skip_package_validation", skipPackageValidation),
		zap.Duration("live_block_time_delta", liveBlockTimeDelta),
		zap.Int("undo_buffer_size", undoBufferSize),
		zap.Strings("extra_headers", extraHeaders),
	)

	var readerOptions []manifest.Option
	protoPath := sflags.MustGetString(cmd, FlagProtoPath)
	if protoPath != "" {
		readerOptions = append(readerOptions, manifest.WithProtoPath(protoPath))
	}

	protoDescriptorSet := sflags.MustGetString(cmd, FlagProtoDescriptorSet)
	if protoDescriptorSet != "" {
		readerOptions = append(readerOptions, manifest.WithProtoDescriptorSet(protoDescriptorSet))
	}

	if outputModuleName != "" {
		readerOptions = append(readerOptions, manifest.WithOverrideOutputModule(outputModuleName))
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

	// Resolve start block if empty (use module's initial block)
	if startBlockIsEmpty {
		startBlock = int64(module.InitialBlock)
	}

	zlog.Debug("resolved block range",
		zap.Int64("start_block", startBlock),
		zap.Uint64("stop_block", stopBlock),
		zap.Bool("start_block_was_empty", startBlockIsEmpty),
	)

	if finalBlocksOnly {
		zlog.Debug("override undo buffer size to 0 since final blocks only is requested")
		undoBufferSize = 0
	}

	auth := newAuthenticator(sflags.MustGetString(cmd, FlagAPIKeyEnvvar), sflags.MustGetString(cmd, FlagAPITokenEnvvar))
	authToken, authType := auth.GetTokenAndType()

	clientConfig := client.NewSubstreamsClientConfig(
		endpoint,
		authToken,
		authType,
		sflags.MustGetBool(cmd, FlagInsecure),
		sflags.MustGetBool(cmd, FlagPlaintext),
		userAgent,
	)

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
		InfiniteRetry:         infiniteRetry,
		BackOff:               bo,
		LiveBlockTimeDelta:    liveBlockTimeDelta,
		LivenessChecker:       livenessChecker,
		ExtraHeaders:          extraHeaders,
		Params:                params,
		Network:               network,
		SkipPackageValidation: skipPackageValidation,
		Logger:                zlog,
		Tracer:                tracer,
	}

	return config, nil
}

func getViperFlags(cmd *cobra.Command) (
	params []string,
	network string,
	undoBufferSize int,
	liveBlockTimeDelta time.Duration,
	isDevelopmentMode bool,
	infiniteRetry bool,
	finalBlocksOnly bool,
	skipPackageValidation bool,
	isNoopMode bool,
	extraHeaders []string,
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

	if sflags.FlagDefined(cmd, FlagInfiniteRetry) {
		infiniteRetry = sflags.MustGetBool(cmd, FlagInfiniteRetry)
	}

	if sflags.FlagDefined(cmd, FlagFinalBlocksOnly) {
		finalBlocksOnly = sflags.MustGetBool(cmd, FlagFinalBlocksOnly)
	}

	if sflags.FlagDefined(cmd, FlagSkipPackageValidation) {
		skipPackageValidation = sflags.MustGetBool(cmd, FlagSkipPackageValidation)
	}

	if sflags.FlagDefined(cmd, FlagExtraHeaders) {
		extraHeaders = sflags.MustGetStringArray(cmd, FlagExtraHeaders)
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
