package sink

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/bobg/go-generics/v2/slices"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/streamingfast/bstream"
	"github.com/streamingfast/cli/sflags"
	"github.com/streamingfast/logging"
	"github.com/streamingfast/substreams/client"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
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
		flags.Int(FlagUndoBufferSize, 12, "Number of blocks to keep buffered to handle fork reorganizations")
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

}

// NewFromViper constructs a new Sinker instance from a fixed set of "known" flags.
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
	endpoint, manifestPath, outputModuleName, blockRange string,
	zlog *zap.Logger,
	tracer logging.Tracer,
	opts ...Option,
) (*Sinker, error) {
	params, network, undoBufferSize, liveBlockTimeDelta, isDevelopmentMode, infiniteRetry, finalBlocksOnly, skipPackageValidation, isNoopMode, extraHeaders := getViperFlags(cmd)

	zlog.Info("sinker from CLI",
		zap.String("endpoint", endpoint),
		zap.String("manifest_path", manifestPath),
		zap.Strings("params", params),
		zap.String("network", network),
		zap.String("output_module_name", outputModuleName),
		zap.Stringer("expected_module_type", expectedModuleType(expectedOutputModuleType)),
		zap.String("block_range", blockRange),
		zap.Bool("development_mode", isDevelopmentMode),
		zap.Bool("noop_mode", isNoopMode),
		zap.Bool("infinite_retry", infiniteRetry),
		zap.Bool("final_blocks_only", finalBlocksOnly),
		zap.Bool("skip_package_validation", skipPackageValidation),
		zap.Duration("live_block_time_delta", liveBlockTimeDelta),
		zap.Int("undo_buffer_size", undoBufferSize),
		zap.Strings("extra_headers", extraHeaders),
	)

	pkg, module, outputModuleHash, resolvedBlockRange, err := ReadManifestAndModuleAndBlockRange(
		manifestPath,
		network,
		params,
		outputModuleName,
		expectedOutputModuleType,
		skipPackageValidation,
		blockRange,
		zlog,
	)
	if err != nil {
		return nil, fmt.Errorf("reading manifest: %w", err)
	}

	zlog.Debug("resolved block range", zap.Stringer("range", resolvedBlockRange))

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
		"sink",
	)

	mode := SubstreamsModeProduction
	if isDevelopmentMode {
		mode = SubstreamsModeDevelopment
	}

	var defaultSinkOptions []Option
	if undoBufferSize > 0 {
		defaultSinkOptions = append(defaultSinkOptions, WithBlockDataBuffer(undoBufferSize))
	}

	if infiniteRetry {
		defaultSinkOptions = append(defaultSinkOptions, WithInfiniteRetry())
	}

	if liveBlockTimeDelta > 0 {
		defaultSinkOptions = append(defaultSinkOptions, WithLivenessChecker(NewDeltaLivenessChecker(liveBlockTimeDelta)))
	}

	if finalBlocksOnly {
		defaultSinkOptions = append(defaultSinkOptions, WithFinalBlocksOnly())
	}

	if resolvedBlockRange != nil {
		defaultSinkOptions = append(defaultSinkOptions, WithBlockRange(resolvedBlockRange))
	}

	if len(extraHeaders) > 0 {
		defaultSinkOptions = append(defaultSinkOptions, WithExtraHeaders(extraHeaders))
	}

	return New(
		mode,
		isNoopMode,
		pkg,
		module,
		outputModuleHash,
		clientConfig,
		zlog,
		tracer,
		append(defaultSinkOptions, opts...)...,
	)
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

type SyncConfig struct {
	Params                []string
	Network               string
	UndoBufferSize        int
	LiveBlockTimeDelta    time.Duration
	IsDevelopmentMode     bool
	InfiniteRetry         bool
	FinalBlocksOnly       bool
	SkipPackageValidation bool
	IsNoopMode            bool
	ExtraHeaders          []string
}

// parseNumber parses a number and indicates whether the number is relative, meaning it starts with a +
func parseNumber(number string) (numberInt64 int64, numberIsEmpty bool, numberIsRelative bool, err error) {
	if number == "" {
		numberIsEmpty = true
		return
	}

	numberIsRelative = strings.HasPrefix(number, "+")
	numberInt64, err = strconv.ParseInt(strings.TrimPrefix(number, "+"), 0, 64)
	if err != nil {
		return 0, false, false, fmt.Errorf("invalid block number value: %w", err)
	}

	return
}

// ReadBlockRange parses a block range string and returns a bstream.Range out of it
// using the model to resolve relative block numbers to absolute block numbers.
//
// The block range string is of the form:
//
//	[<before>]:[<after>]
//
// Where before and after are block numbers. If before is empty, it is resolve to the module's start block.
// If after is empty it means stream forever. If after is empty and before is empty, the
// range is the entire chain.
//
// If before or after is prefixed with a +, it is relative to the module's start block.
func ReadBlockRange(module *pbsubstreams.Module, input string) (*bstream.Range, error) {
	if input == "" {
		input = ":"
	}

	before, after, rangeHasStartAndStop := strings.Cut(input, ":")

	beforeAsInt64, beforeIsEmpty, beforeIsRelative, err := parseNumber(before)
	if err != nil {
		return nil, fmt.Errorf("parse number %q: %w", before, err)
	}

	if beforeIsEmpty {
		// We become automatically relative to module start block with +0, so we get back module's start block
		beforeIsRelative = true
	}

	afterAsInt64, afterIsEmpty, afterIsRelative := int64(0), false, false
	if rangeHasStartAndStop {
		afterAsInt64, afterIsEmpty, afterIsRelative, err = parseNumber(after)
		if err != nil {
			return nil, fmt.Errorf("parse number %q: %w", after, err)
		}
	}

	if !rangeHasStartAndStop {
		// If there is no `:` we assume it's a stop block value right away
		if beforeAsInt64 < 1 {
			return bstream.NewOpenRange(module.InitialBlock), nil
		}

		start := module.InitialBlock
		stop := resolveBlockNumber(beforeAsInt64, 0, beforeIsRelative, int64(start))

		if int64(start) >= stop {
			return nil, fmt.Errorf("invalid range: start block %d is equal or above stop block %d (exclusive)", start, stop)
		}

		return bstream.NewRangeExcludingEnd(start, uint64(stop)), nil
	} else {
		// Otherwise, we have a `:` sign so we assume it's a start/stop range
		start := resolveBlockNumber(beforeAsInt64, int64(module.InitialBlock), beforeIsRelative, int64(module.InitialBlock))
		if afterAsInt64 == -1 {
			return bstream.NewOpenRange(uint64(start)), nil
		}

		startBlock := uint64(start)
		if afterIsEmpty {
			return bstream.NewOpenRange(startBlock), nil
		}

		exclusiveEndBlock := uint64(resolveBlockNumber(afterAsInt64, 0, afterIsRelative, start))

		if startBlock >= exclusiveEndBlock {
			return nil, fmt.Errorf("invalid range: start block %d is equal or above stop block %d (exclusive)", startBlock, exclusiveEndBlock)
		}

		return bstream.NewRangeExcludingEnd(startBlock, exclusiveEndBlock), nil
	}
}

func resolveBlockNumber(value int64, defaultIfNegative int64, relative bool, against int64) int64 {
	if !relative {
		if value < 0 {
			return defaultIfNegative
		}
		return value
	}
	return int64(against) + value
}

func every[E any](s []E, test func(e E) bool) bool {
	for _, element := range s {
		if !test(element) {
			return false
		}
	}

	return true
}
