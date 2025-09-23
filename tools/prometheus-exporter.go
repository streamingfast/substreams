package tools

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

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
	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
	pbsubstreamsrpcv3 "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v3"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

var lastStatus = map[string]bool{}
var lock = &sync.Mutex{}

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
	prometheusCmd.Flags().Bool("force-v2", false, "Force using V2 API")
	prometheusCmd.Flags().Int64("block-height", -1, "Block number to request (defaults to -1, which means the HEAD)")
	prometheusCmd.Flags().Duration("max-freshness", time.Minute*2, "(only used if block-height is relative, i.e. below 0) check the age of the received blocks, fail an endpoint if it is older than this duration")
	prometheusCmd.Flags().Duration("interval", time.Second*20, "endpoints will be polled at this interval")
	prometheusCmd.Flags().Duration("timeout", time.Second*10, "endpoints will be considered 'failing' if they don't complete in that duration")

	Cmd.AddCommand(prometheusCmd)
}

var status *prometheus.GaugeVec
var requestDurationMs *prometheus.GaugeVec
var blockAgeMs *prometheus.GaugeVec
var endpointMap = make(map[string]endpointSpecs)

type endpointSpecs struct {
	url        string
	startBlock *int
	labels     prometheus.Labels
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
	for _, part := range strings.Split(in, "&") {
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
	timeout := sflags.MustGetDuration(cmd, "timeout")
	forceV2 := sflags.MustGetBool(cmd, "force-v2")
	maxFreshness := sflags.MustGetDuration(cmd, "max-freshness")

	allLabels := map[string]bool{"endpoint": true}

	for _, endpoint := range endpoints {
		url, startBlock, params, err := parseEndpoint(endpoint)
		if err != nil {
			return fmt.Errorf("invalid endpoint %q: %w", endpoint, err)
		}

		labels := prometheus.Labels{"endpoint": url}
		for k, v := range params {
			labels[k] = v
			allLabels[k] = true
		}
		endpointMap[url] = endpointSpecs{url: url, startBlock: startBlock, labels: labels}
	}

	allLabelsSlice := make([]string, 0, len(allLabels))
	for k := range allLabels {
		allLabelsSlice = append(allLabelsSlice, k)
	}

	status = prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "substreams_healthcheck_status", Help: "Either 1 for successful subtreams request, or 0 for failure"}, allLabelsSlice)
	requestDurationMs = prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "substreams_healthcheck_duration_ms", Help: "Request full processing time in millisecond"}, allLabelsSlice)
	blockAgeMs = prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "substreams_healthcheck_block_age_ms", Help: "Age of returned block"}, allLabelsSlice)

	for endpoint := range endpointMap {
		startBlock := blockNum
		if endpointMap[endpoint].startBlock != nil {
			startBlock = int64(*endpointMap[endpoint].startBlock)
		}

		substreamsClientConfig := client.NewSubstreamsClientConfig(
			endpoint,
			authToken,
			authType,
			insecure,
			plaintext,
			"substreams_prometheus",
			forceV2,
		)
		var fresh *time.Duration
		if startBlock < 0 {
			fresh = &maxFreshness
		}

		go launchSubstreamsPoller(endpoint, substreamsClientConfig, pkgBundle.Package, outputStreamName, startBlock, interval, timeout, fresh)
	}

	promReg := prometheus.NewRegistry()
	promReg.MustRegister(status)
	promReg.MustRegister(requestDurationMs)
	promReg.MustRegister(blockAgeMs)

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

func markSuccess(endpoint string, begin time.Time) {
	lock.Lock()
	defer lock.Unlock()
	if !lastStatus[endpoint] {
		zlog.Info("endpoint now marked as available", zap.String("endpoint", endpoint))
	}
	lastStatus[endpoint] = true
	status.With(endpointMap[endpoint].labels).Set(1)
	requestDurationMs.With(endpointMap[endpoint].labels).Set(float64(time.Since(begin).Milliseconds()))
}

func markFailure(endpoint string, begin time.Time, err error) {
	lock.Lock()
	defer lock.Unlock()
	if val, ok := lastStatus[endpoint]; !ok || val {
		zlog.Info("endpoint now marked as unavailable", zap.String("endpoint", endpoint), zap.Error(err))
		lastStatus[endpoint] = false
	}
	status.With(endpointMap[endpoint].labels).Set(0)
	requestDurationMs.With(endpointMap[endpoint].labels).Set(float64(time.Since(begin).Milliseconds()))
}

func launchSubstreamsPoller(endpoint string, substreamsClientConfig *client.SubstreamsClientConfig, pkg *pbsubstreams.Package, outputStreamName string, blockNum int64, pollingInterval, pollingTimeout time.Duration, maxFreshness *time.Duration) {

	sleep := time.Duration(0)
	for {
		time.Sleep(sleep)
		sleep = pollingInterval

		ctx, cancel := context.WithTimeout(context.Background(), pollingTimeout)
		begin := time.Now()
		ssClientV2, ssClientV3, connClose, callOpts, headers, err := client.NewSubstreamsClients(substreamsClientConfig)
		if err != nil {
			zlog.Error("substreams client setup", zap.Error(err))
			markFailure(endpoint, begin, err)
			cancel()
			continue
		}

		if headers.IsSet() {
			ctx = metadata.AppendToOutgoingContext(ctx, headers.ToArray()...)
		}

		var stopBlockNum uint64
		if blockNum > 0 {
			stopBlockNum = uint64(blockNum + 1)
		}
		subReq := &pbsubstreamsrpcv3.Request{
			StartBlockNum: blockNum,
			StopBlockNum:  stopBlockNum,
			Package:       pkg,
			OutputModule:  outputStreamName,
		}

		if err := subReq.Validate(); err != nil {
			zlog.Error("validate request", zap.Error(err))
			markFailure(endpoint, begin, err)
			connClose()
			cancel()
			continue
		}
		callOpts = append(callOpts, grpc.WaitForReady(false))
		zlog.Debug("calling sf.substreams.rpc.v2.Stream/Blocks", zap.String("endpoint", endpoint), zap.String("output_module", outputStreamName), zap.Int64("start_block", blockNum), zap.Uint64("stop_block", stopBlockNum))

		var cli grpc.ServerStreamingClient[pbsubstreamsrpc.Response]
		if substreamsClientConfig.ForceV2() {
			cli, err = ssClientV2.Blocks(ctx, subReq.ToV2(), callOpts...)
			if err != nil {
				zlog.Error("call sf.substreams.rpc.v2.Stream/Blocks", zap.String("endpoint", endpoint), zap.Error(err))
				markFailure(endpoint, begin, err)
				connClose()
				cancel()
				continue
			}
		}
		cli, err = ssClientV3.Blocks(ctx, subReq, callOpts...)
		if err != nil {
			zlog.Error("call sf.substreams.rpc.v2.Stream/Blocks", zap.String("endpoint", endpoint), zap.Error(err))
			markFailure(endpoint, begin, err)
			connClose()
			cancel()
			continue
		}

	forloop:
		for {
			resp, err := cli.Recv()
			if resp != nil {
				switch resp.Message.(type) {
				case *pbsubstreamsrpc.Response_BlockScopedData:
					if maxFreshness == nil {
						zlog.Debug("marking endpoint with success",
							zap.String("endpoint", endpoint),
							zap.Duration("duration", time.Since(begin)),
							zap.Uint64("block_num", resp.Message.(*pbsubstreamsrpc.Response_BlockScopedData).BlockScopedData.Clock.Number),
						)
						markSuccess(endpoint, begin)
						break forloop
					}
					blockTime := resp.Message.(*pbsubstreamsrpc.Response_BlockScopedData).BlockScopedData.Clock.Timestamp.AsTime()
					blockAgeMs.With(endpointMap[endpoint].labels).Set(float64(time.Since(blockTime).Milliseconds()))
					if age := time.Since(blockTime); age > *maxFreshness {
						markFailure(endpoint, begin, fmt.Errorf("block is too old: %s", age))
						zlog.Debug("marking endpoint with failure because of freshness", zap.String("endpoint", endpoint), zap.Duration("duration", time.Since(begin)), zap.Duration("block_age", time.Since(blockTime)))
					} else {
						markSuccess(endpoint, begin)
						zlog.Debug("marking endpoint with success",
							zap.String("endpoint", endpoint),
							zap.Duration("duration", time.Since(begin)),
							zap.Duration("block_age", time.Since(blockTime)),
							zap.Uint64("block_num", resp.Message.(*pbsubstreamsrpc.Response_BlockScopedData).BlockScopedData.Clock.Number),
						)
					}
					break forloop
				}
			}
			if err != nil {
				markFailure(endpoint, begin, err)
				break
			}
		}

		connClose()
		cancel()
	}
}
