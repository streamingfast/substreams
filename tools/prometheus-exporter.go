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
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

var status = prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "substreams_healthcheck_status", Help: "Either 1 for successful subtreams request, or 0 for failure"}, []string{"endpoint"})
var requestDurationMs = prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "substreams_healthcheck_duration_ms", Help: "Request full processing time in millisecond"}, []string{"endpoint"})
var blockAgeMs = prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "substreams_healthcheck_block_age_ms", Help: "Age of returned block"}, []string{"endpoint"})

var lastStatus = map[string]bool{}
var lock = &sync.Mutex{}

var prometheusCmd = &cobra.Command{
	Use:   "prometheus-exporter <endpoint[,endpoint[,endpoint[@<block_height>]],[,...]]> <manifest> <module_name>",
	Short: "run substreams client periodically on a single block, exporting the values in prometheus format",
	Long: cli.Dedent(`
		Run substreams client periodically on a single block, exporting the values in prometheus format.
	    The manifest can be a local file or a URL to an spkg.
        You can specify a start-block on some endpoints by appending '@<block_height>' to the endpoint URL.
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
	prometheusCmd.Flags().Int64("block-height", -1, "Block number to request (defaults to -1, which means the HEAD)")
	prometheusCmd.Flags().Duration("max-freshness", time.Minute*2, "(only used if block-height is relative, i.e. below 0) check the age of the received blocks, fail an endpoint if it is older than this duration")
	prometheusCmd.Flags().Duration("interval", time.Second*20, "endpoints will be polled at this interval")
	prometheusCmd.Flags().Duration("timeout", time.Second*10, "endpoints will be considered 'failing' if they don't complete in that duration")

	Cmd.AddCommand(prometheusCmd)
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
	maxFreshness := sflags.MustGetDuration(cmd, "max-freshness")
	for _, endpoint := range endpoints {
		startBlock := blockNum
		if parts := strings.Split(endpoint, "@"); len(parts) == 2 {
			start, err := strconv.ParseInt(parts[1], 10, 64)
			if err != nil {
				return fmt.Errorf("invalid endpoint @startBlock format for %q: %w", endpoint, err)
			}
			endpoint = parts[0]
			startBlock = start
		}

		substreamsClientConfig := client.NewSubstreamsClientConfig(
			endpoint,
			authToken,
			authType,
			insecure,
			plaintext,
		)
		var fresh *time.Duration
		if startBlock < 0 {
			fresh = &maxFreshness
		}

		go launchSubstreamsPoller(endpoint, substreamsClientConfig, pkgBundle.Package.Modules, outputStreamName, startBlock, interval, timeout, fresh)
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
	status.With(prometheus.Labels{"endpoint": endpoint}).Set(1)
	requestDurationMs.With(prometheus.Labels{"endpoint": endpoint}).Set(float64(time.Since(begin).Milliseconds()))
}

func markFailure(endpoint string, begin time.Time, err error) {
	lock.Lock()
	defer lock.Unlock()
	if val, ok := lastStatus[endpoint]; !ok || val {
		zlog.Info("endpoint now marked as unavailable", zap.String("endpoint", endpoint), zap.Error(err))
		lastStatus[endpoint] = false
	}
	status.With(prometheus.Labels{"endpoint": endpoint}).Set(0)
	requestDurationMs.With(prometheus.Labels{"endpoint": endpoint}).Set(float64(time.Since(begin).Milliseconds()))
}

func launchSubstreamsPoller(endpoint string, substreamsClientConfig *client.SubstreamsClientConfig, modules *pbsubstreams.Modules, outputStreamName string, blockNum int64, pollingInterval, pollingTimeout time.Duration, maxFreshness *time.Duration) {

	sleep := time.Duration(0)
	for {
		time.Sleep(sleep)
		sleep = pollingInterval

		ctx, cancel := context.WithTimeout(context.Background(), pollingTimeout)
		begin := time.Now()
		ssClient, connClose, callOpts, headers, err := client.NewSubstreamsClient(substreamsClientConfig)
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
		subReq := &pbsubstreamsrpc.Request{
			StartBlockNum: blockNum,
			StopBlockNum:  stopBlockNum,
			Modules:       modules,
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
		cli, err := ssClient.Blocks(ctx, subReq, callOpts...)
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
					blockAgeMs.With(prometheus.Labels{"endpoint": endpoint}).Set(float64(time.Since(blockTime).Milliseconds()))
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
