package tools

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"math"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/metadata"

	"github.com/streamingfast/cli"
	"github.com/streamingfast/cli/sflags"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"google.golang.org/grpc"

	"github.com/streamingfast/substreams/client"
	"github.com/streamingfast/substreams/manifest"
	pbsubstreamsrpcv3 "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v3"
	pbsubstreamsrpcv4 "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v4"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

var prometheusCmd = &cobra.Command{
	Use:   "prometheus-exporter <endpoint[,endpoint[,endpoint[@<block_height>]],[,...]]> <manifest> <module_name>",
	Short: "run substreams client periodically on a single block, exporting the values in prometheus format",
	Long: cli.Dedent(`
		Run substreams client periodically on a single block, exporting the values in prometheus format.
	    The manifest can be a local file or a URL to an spkg.
        You can specify a start-block on some endpoints by appending '@<block_height>' to the endpoint URL.
        You can specify additional prometheus labels as query parameters, e.g. 'my.domain:443?namespace=eth-mainnet'.
        Specify both togethers like this: 'my.domain:443@-10?namespace=eth-mainnet&region=us-east'.
	`),
	RunE:         runPrometheus,
	Args:         cobra.ExactArgs(3),
	SilenceUsage: true,
}

func init() {
	prometheusCmd.Flags().String("listen-addr", ":9102", "prometheus listen address")
	prometheusCmd.Flags().String("substreams-api-token-envvar", "SUBSTREAMS_API_TOKEN", "name of variable containing Substreams Authentication token")
	prometheusCmd.Flags().String("substreams-api-key-envvar", "SUBSTREAMS_API_KEY", "Name of variable containing Substreams Api Key")
	prometheusCmd.Flags().BoolP("insecure", "k", false, "Skip certificate validation on GRPC connection")
	prometheusCmd.Flags().BoolP("plaintext", "p", false, "Establish GRPC connection in plaintext")
	prometheusCmd.Flags().Int("force-protocol-version", 0, "Force the use of a specific protocol version (0=unset, 4=v4), only v4 is accepted for now, the flag is kept for the next protocol versions")
	prometheusCmd.Flags().Int64("block-height", -1, "Block number to request (defaults to -1, which means the HEAD)")
	prometheusCmd.Flags().Duration("max-freshness", time.Minute*2, "(only used if block-height is relative, i.e. below 0) check the age of the received blocks, fail an endpoint if it is older than this duration")
	prometheusCmd.Flags().Duration("interval", time.Second*20, "endpoints will be polled at this interval")
	prometheusCmd.Flags().Duration("connect-timeout", time.Second*10, "endpoints will be considered 'failing' (with reason 'connect_timeout') if the gRPC connection does not become ready in that duration, this budget is separate from --timeout")
	prometheusCmd.Flags().Duration("timeout", time.Second*10, "endpoints will be considered 'failing' if the Blocks request does not complete in that duration, this excludes the time spent establishing the connection (see --connect-timeout)")

	Cmd.AddCommand(prometheusCmd)
}

var endpointMap = make(map[string]endpointSpecs)

type endpointSpecs struct {
	startBlock *int
	// labelValues holds one value per metric label name, in the same order. Prometheus
	// requires a value for every declared label, so an endpoint given fewer query parameters
	// than another one gets an empty value for the labels it is missing.
	labelValues []string
}

// endpointState tracks what we already reported about an endpoint so that we can log
// both the state transitions and the individual failures, with enough context to tell a
// single hiccup apart from a sustained outage.
type endpointState struct {
	known               bool
	available           bool
	since               time.Time
	consecutiveFailures int
	blockAgeAboveHalf   bool
	blockAgeStreak      int
}

var endpointStates = map[string]*endpointState{}
var lock = &sync.Mutex{}

// stateFor must be called with `lock` held.
func stateFor(endpoint string) *endpointState {
	state, found := endpointStates[endpoint]
	if !found {
		state = &endpointState{since: time.Now()}
		endpointStates[endpoint] = state
	}
	return state
}

func extractStartblock(in string) (prefix string, startBlock *int, err error) {
	parts := strings.SplitN(in, "@", 2)
	switch len(parts) {
	case 1:
		return in, nil, nil
	case 2:
		prefix = parts[0]
		var start int

		start, err = strconv.Atoi(parts[1])
		if err != nil {
			return "", nil, err
		}
		startBlock = &start
		return
	}
	return "", nil, fmt.Errorf("invalid endpoint part: %q", in)
}

func extractParams(in string) (params map[string]string, err error) {
	if in == "" {
		return nil, nil
	}

	params = make(map[string]string)
	for part := range strings.SplitSeq(in, "&") {
		parts := strings.SplitN(part, "=", 2)
		switch len(parts) {
		case 0:
			return nil, fmt.Errorf("invalid param format: %q", part)
		case 1:
			params[parts[0]] = ""
		case 2:
			params[parts[0]] = parts[1]
		}
	}
	return params, nil
}

func parseEndpoint(in string) (endpoint string, startBlock *int, params map[string]string, err error) {
	parts := strings.SplitN(in, "?", 2)

	switch len(parts) {
	case 1:
		endpoint, startBlock, err = extractStartblock(parts[0])
		if err != nil {
			return
		}
		return
	case 2:
		endpoint = parts[0]
		paramsString := parts[1]

		if strings.Contains(endpoint, "@") {
			endpoint, startBlock, err = extractStartblock(endpoint)
			if err != nil {
				return
			}
		} else if strings.Contains(paramsString, "@") {
			paramsString, startBlock, err = extractStartblock(paramsString)
			if err != nil {
				return
			}
		}
		params, err = extractParams(paramsString)
		return
	}

	return "", nil, nil, fmt.Errorf("invalid endpoint format: %q", in)
}

func runPrometheus(cmd *cobra.Command, args []string) error {

	endpoints := strings.Split(args[0], ",")
	manifestPath := args[1]
	moduleName := args[2]

	blockNum := sflags.MustGetInt64(cmd, "block-height")
	addr := sflags.MustGetString(cmd, "listen-addr")

	manifestReader, err := manifest.NewReader(manifestPath)
	if err != nil {
		return fmt.Errorf("manifest reader: %w", err)
	}

	pkgBundle, err := manifestReader.Read()
	if err != nil {
		return fmt.Errorf("reading manifest %q: %w", manifestPath, err)
	}

	if pkgBundle == nil {
		return fmt.Errorf("no package found")
	}

	outputStreamName := moduleName

	authToken, authType := GetAuth(cmd, "substreams-api-key-envvar", "substreams-api-token-envvar")
	insecure := sflags.MustGetBool(cmd, "insecure")
	plaintext := sflags.MustGetBool(cmd, "plaintext")
	interval := sflags.MustGetDuration(cmd, "interval")
	connectTimeout := sflags.MustGetDuration(cmd, "connect-timeout")
	timeout := sflags.MustGetDuration(cmd, "timeout")

	// Checked before `ParseProtocolVersion` so that an operator passing 2 or 3 reads the one
	// message that applies here, rather than being told v2 and v3 are supported and then
	// refused them on the next line.
	protocolVersionFlag := sflags.MustGetInt(cmd, "force-protocol-version")
	if protocolVersionFlag != 0 && protocolVersionFlag != int(client.ProtocolVersionV4) {
		return fmt.Errorf("invalid --force-protocol-version %d: the prometheus exporter only speaks %s for now, leave the flag unset or pass 4", protocolVersionFlag, client.ProtocolVersionV4)
	}

	// The check above leaves only 0 and 4, both of which parse.
	forceProtocolVersion, _ := client.ParseProtocolVersion(protocolVersionFlag)

	maxFreshness := sflags.MustGetDuration(cmd, "max-freshness")

	type parsedEndpoint struct {
		url        string
		startBlock *int
		params     map[string]string
	}

	allLabels := map[string]bool{endpointLabel: true}
	parsed := make([]parsedEndpoint, 0, len(endpoints))

	for _, endpoint := range endpoints {
		url, startBlock, params, err := parseEndpoint(endpoint)
		if err != nil {
			return fmt.Errorf("invalid endpoint %q: %w", endpoint, err)
		}

		for k := range params {
			allLabels[k] = true
		}
		parsed = append(parsed, parsedEndpoint{url: url, startBlock: startBlock, params: params})
	}

	labelNames := slices.Sorted(maps.Keys(allLabels))

	for _, endpoint := range parsed {
		endpointMap[endpoint.url] = endpointSpecs{
			startBlock:  endpoint.startBlock,
			labelValues: endpointLabelValues(endpoint.url, endpoint.params, labelNames),
		}
	}

	// Declared before the pollers start: a poller that fails on its very first attempt already
	// reports through these, and they are read without synchronisation.
	collectors := initHealthcheckMetrics(labelNames)

	for endpoint := range endpointMap {
		startBlock := blockNum
		if endpointMap[endpoint].startBlock != nil {
			startBlock = int64(*endpointMap[endpoint].startBlock)
		}

		substreamsClientConfig := client.NewSubstreamsClientConfig(client.SubstreamsClientConfigOptions{
			Endpoint:             endpoint,
			AuthToken:            authToken,
			AuthType:             authType,
			Insecure:             insecure,
			PlainText:            plaintext,
			Agent:                "substreams_prometheus",
			ForceProtocolVersion: forceProtocolVersion,
		})
		var fresh *time.Duration
		if startBlock < 0 {
			fresh = &maxFreshness
		}

		go launchSubstreamsPoller(endpoint, substreamsClientConfig, pkgBundle.Package, outputStreamName, startBlock, interval, connectTimeout, timeout, fresh)
	}

	// The exporter serves only its own metrics, so the collectors go to a dedicated registry
	// instead of the global one that `dmetrics.Set.Register` would use.
	promReg := prometheus.NewRegistry()
	promReg.MustRegister(collectors...)

	handler := promhttp.HandlerFor(
		promReg,
		promhttp.HandlerOpts{
			EnableOpenMetrics: false,
		})

	serve := http.Server{Handler: handler, Addr: addr}
	if err := serve.ListenAndServe(); err != nil {
		zlog.Info("can't listen on the metrics endpoint", zap.Error(err))
		return err
	}
	return nil
}

func markSuccess(endpoint string, result *pollResult) {
	lock.Lock()
	defer lock.Unlock()

	state := stateFor(endpoint)
	if !state.known || !state.available {
		fields := []zap.Field{
			zap.String("endpoint", endpoint),
			zap.Duration("connect_duration", result.connectDuration),
			zap.Duration("stream_duration", result.streamDuration),
		}
		if state.known {
			fields = append(fields,
				zap.Duration("unavailable_for", time.Since(state.since)),
				zap.Int("failed_polls", state.consecutiveFailures),
			)
		}
		zlog.Info("endpoint now marked as available", fields...)

		state.known = true
		state.available = true
		state.since = time.Now()
	}
	state.consecutiveFailures = 0
	trackBlockAge(endpoint, state, result)

	labelValues := endpointMap[endpoint].labelValues
	status.SetInt(1, labelValues...)
	consecutiveFailures.SetInt(0, labelValues...)
	requestDurationMs.SetInt64(result.totalDuration().Milliseconds(), labelValues...)
	connectDurationMs.SetInt64(result.connectDuration.Milliseconds(), labelValues...)
	streamDurationMs.SetInt64(result.streamDuration.Milliseconds(), labelValues...)
	if result.blockAge != nil {
		blockAgeMs.SetInt64(result.blockAge.Milliseconds(), labelValues...)
	}
}

func markFailure(endpoint string, result *pollResult) {
	lock.Lock()
	defer lock.Unlock()

	state := stateFor(endpoint)
	state.consecutiveFailures++

	grpcCode := grpcCodeOf(result.err)
	fields := []zap.Field{
		zap.String("endpoint", endpoint),
		zap.String("reason", string(result.reason)),
		zap.String("grpc_code", grpcCode),
		zap.Duration("connect_duration", result.connectDuration),
		zap.Duration("stream_duration", result.streamDuration),
		zap.Error(result.err),
	}

	if !state.known || state.available {
		if state.known {
			fields = append(fields, zap.Duration("available_for", time.Since(state.since)))
		}
		zlog.Info("endpoint now marked as unavailable", fields...)

		state.known = true
		state.available = false
		state.since = time.Now()
	} else {
		// An endpoint that fails repeatedly, or one that flaps between two Prometheus scrapes,
		// stays invisible in the logs unless every failure is reported, not just the first one.
		fields = append(fields,
			zap.Int("consecutive_failures", state.consecutiveFailures),
			zap.Duration("unavailable_for", time.Since(state.since)),
		)
		zlog.Info("endpoint poll failed", fields...)
	}

	trackBlockAge(endpoint, state, result)

	labelValues := endpointMap[endpoint].labelValues
	status.SetInt(0, labelValues...)
	consecutiveFailures.SetInt(state.consecutiveFailures, labelValues...)
	requestDurationMs.SetInt64(result.totalDuration().Milliseconds(), labelValues...)
	connectDurationMs.SetInt64(result.connectDuration.Milliseconds(), labelValues...)
	streamDurationMs.SetInt64(result.streamDuration.Milliseconds(), labelValues...)
	failureCount.Inc(failureLabelValues(endpoint, result.reason, grpcCode)...)

	if result.blockAge != nil {
		blockAgeMs.SetInt64(result.blockAge.Milliseconds(), labelValues...)
	} else {
		// Without this, the gauge keeps reporting the age of the last block we ever saw, which
		// silently gets younger than reality the longer the endpoint stays broken.
		blockAgeMs.SetFloat64(math.NaN(), labelValues...)
	}
}

// blockAgeFlipPolls is how many consecutive polls must agree before the block-age report
// flips. A chain whose block interval straddles half of --max-freshness crosses the threshold
// on almost every poll, so a bare edge trigger reports a healthy endpoint forever, just in
// pairs of lines instead of one. Requiring a streak reports only a drift that persists.
const blockAgeFlipPolls = 3

// trackBlockAge reports the block age on the crossings rather than on every poll, the way the
// availability transitions are reported: an age sitting just above the threshold is the normal
// state of a chain whose block interval is close to it, and saying so once per poll drowns the
// signal. Must be called with `lock` held.
func trackBlockAge(endpoint string, state *endpointState, result *pollResult) {
	if result.blockAge == nil || result.maxFreshness == nil {
		// A poll that carries no age agrees with nothing, so a streak cannot span it.
		state.blockAgeStreak = 0
		return
	}

	aboveHalf := *result.blockAge > *result.maxFreshness/2
	if aboveHalf == state.blockAgeAboveHalf {
		state.blockAgeStreak = 0
		return
	}

	state.blockAgeStreak++
	if state.blockAgeStreak < blockAgeFlipPolls {
		return
	}

	state.blockAgeAboveHalf = aboveHalf
	state.blockAgeStreak = 0

	message := "endpoint block age fell back below half of the max freshness"
	if aboveHalf {
		// This is what precedes a `stale_block` failure, and what makes an alert on
		// `block_age_ms` explainable.
		message = "endpoint block age climbed above half of the max freshness"
	}

	zlog.Info(message,
		zap.String("endpoint", endpoint),
		zap.Duration("block_age", *result.blockAge),
		zap.Duration("max_freshness", *result.maxFreshness),
		zap.Int("confirmed_over_polls", blockAgeFlipPolls),
	)
}

// pollResult is the outcome of a single poll, `err` being nil means the endpoint is healthy.
type pollResult struct {
	connectDuration time.Duration
	streamDuration  time.Duration
	blockAge        *time.Duration
	maxFreshness    *time.Duration
	reason          failureReason
	err             error
}

func (r *pollResult) totalDuration() time.Duration {
	return r.connectDuration + r.streamDuration
}

// errConnDialFailed reports that the channel failed a dial before the connect budget ran out.
// The connectivity API never hands over the dial error, so the caller issues the request
// anyway and lets gRPC answer with it.
var errConnDialFailed = errors.New("connection failed to establish")

// waitForConnReady blocks until the gRPC channel is usable. gRPC dials lazily, so without
// this the DNS resolution, the TLS handshake and the load-balancer setup would all be
// charged to the Blocks request budget, and every slow connection would be reported as an
// endpoint failure ("waiting for new LB policy update: context deadline exceeded").
//
// A failed dial does not end the attempt. gRPC re-dials on its own backoff and the connect
// budget exists to cover a backend that is restarting, so an endpoint that comes back inside
// the budget is healthy and must be reported as such. What the failed dial does change is how
// the timeout is described: `errConnDialFailed` says the endpoint refused us rather than
// being slow, which the caller turns into the real dial error.
func waitForConnReady(ctx context.Context, conn *grpc.ClientConn) error {
	conn.Connect()

	dialFailed := false
	for {
		state := conn.GetState()
		switch state {
		case connectivity.Ready:
			return nil
		case connectivity.Shutdown:
			return fmt.Errorf("connection shut down before becoming ready")
		case connectivity.TransientFailure:
			dialFailed = true
		}

		if !conn.WaitForStateChange(ctx, state) {
			if dialFailed {
				return errConnDialFailed
			}
			return fmt.Errorf("connection stuck in state %q: %w", state, context.Cause(ctx))
		}
	}
}

func launchSubstreamsPoller(endpoint string, substreamsClientConfig *client.SubstreamsClientConfig, pkg *pbsubstreams.Package, outputStreamName string, blockNum int64, pollingInterval, connectTimeout, pollingTimeout time.Duration, maxFreshness *time.Duration) {
	sleep := time.Duration(0)
	for {
		time.Sleep(sleep)
		sleep = pollingInterval

		result := pollEndpoint(endpoint, substreamsClientConfig, pkg, outputStreamName, blockNum, connectTimeout, pollingTimeout, maxFreshness)
		if result.err != nil {
			markFailure(endpoint, result)
			continue
		}
		markSuccess(endpoint, result)
	}
}

func pollEndpoint(endpoint string, substreamsClientConfig *client.SubstreamsClientConfig, pkg *pbsubstreams.Package, outputStreamName string, blockNum int64, connectTimeout, pollingTimeout time.Duration, maxFreshness *time.Duration) (result *pollResult) {
	result = &pollResult{maxFreshness: maxFreshness}

	connectBegin := time.Now()
	conn, connClose, callOpts, headers, err := client.NewSubstreamsClientConn(substreamsClientConfig)
	if err != nil {
		result.connectDuration = time.Since(connectBegin)
		result.reason, result.err = reasonInvalidConfig, err
		return
	}
	defer connClose()

	connectCtx, cancelConnect := context.WithTimeoutCause(context.Background(), connectTimeout, fmt.Errorf("connect timeout of %s reached", connectTimeout))
	defer cancelConnect()

	var connectFailed bool
	if err := waitForConnReady(connectCtx, conn); err != nil {
		if !errors.Is(err, errConnDialFailed) {
			result.connectDuration = time.Since(connectBegin)
			result.reason, result.err = reasonConnectTimeout, err
			return
		}

		// The dial failed, and only the request will say why: gRPC answers it immediately with
		// the dial error it is holding ("connection refused", "no such host"), which is exactly
		// the information an operator needs and which the connectivity API does not expose.
		connectFailed = true
	}
	result.connectDuration = time.Since(connectBegin)

	streamBegin := time.Now()
	defer func() { result.streamDuration = time.Since(streamBegin) }()

	ctx, cancel := context.WithTimeoutCause(context.Background(), pollingTimeout, fmt.Errorf("request timeout of %s reached", pollingTimeout))
	defer cancel()

	if headers.IsSet() {
		ctx = metadata.AppendToOutgoingContext(ctx, headers.ToArray()...)
	}

	var stopBlockNum uint64
	if blockNum > 0 {
		stopBlockNum = uint64(blockNum + 1)
	}

	// `sf.substreams.rpc.v4.Stream/Blocks` takes a v3 request and answers with v4 responses.
	subReq := &pbsubstreamsrpcv3.Request{
		StartBlockNum: blockNum,
		StopBlockNum:  stopBlockNum,
		Package:       pkg,
		OutputModule:  outputStreamName,
	}

	if err := subReq.Validate(); err != nil {
		result.reason, result.err = reasonInvalidRequest, err
		return
	}

	// Fail fast: the connection is either READY or known-broken by now, so waiting for it here
	// would only re-do what the connect phase already decided.
	callOpts = append(callOpts, grpc.WaitForReady(false))
	zlog.Debug("calling sf.substreams.rpc.v4.Stream/Blocks", zap.String("endpoint", endpoint), zap.String("output_module", outputStreamName), zap.Int64("start_block", blockNum), zap.Uint64("stop_block", stopBlockNum), zap.Duration("connect_duration", result.connectDuration))

	streamClient, err := pbsubstreamsrpcv4.NewStreamClient(conn).Blocks(ctx, subReq, callOpts...)
	if err != nil {
		result.reason, result.err = streamFailure(ctx, connectFailed, err)
		return
	}

	for {
		resp, err := streamClient.Recv()
		if err != nil {
			result.reason, result.err = streamFailure(ctx, connectFailed, err)
			return
		}

		data, ok := resp.Message.(*pbsubstreamsrpcv4.Response_BlockScopedDatas)
		if !ok || len(data.BlockScopedDatas.Items) == 0 {
			continue
		}

		// Items are ordered by block number ascending, the last one is the freshest, which is
		// what a HEAD healthcheck cares about.
		clock := data.BlockScopedDatas.Items[len(data.BlockScopedDatas.Items)-1].Clock
		if clock == nil {
			// Nothing recovers a poller, so an unguarded dereference here would take down the
			// exporter for every other endpoint too.
			result.reason = reasonInvalidResponse
			result.err = fmt.Errorf("endpoint returned block data without a clock")
			return
		}

		if maxFreshness == nil {
			zlog.Debug("marking endpoint with success", zap.String("endpoint", endpoint), zap.Uint64("block_num", clock.Number))
			return
		}

		age := time.Since(clock.Timestamp.AsTime())
		result.blockAge = &age
		if age > *maxFreshness {
			result.reason = reasonStaleBlock
			result.err = fmt.Errorf("block %d is too old: %s, above the %s max freshness", clock.Number, age, *maxFreshness)
			return
		}

		zlog.Debug("marking endpoint with success", zap.String("endpoint", endpoint), zap.Uint64("block_num", clock.Number), zap.Duration("block_age", age))
		return
	}
}
