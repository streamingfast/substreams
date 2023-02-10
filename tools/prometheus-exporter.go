package tools

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/streamingfast/cli"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/cobra"
	"github.com/streamingfast/substreams/client"
	"github.com/streamingfast/substreams/manifest"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"go.uber.org/zap"
	"google.golang.org/grpc"
)

var status = prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "substreams_healthcheck_status", Help: "Either 1 for successful subtreams request, or 0 for failure"}, []string{"endpoint"})
var requestDurationMs = prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "substreams_healthcheck_duration_ms", Help: "Request full processing time in millisecond"}, []string{"endpoint"})

var prometheusCmd = &cobra.Command{
	Use:   "prometheus-exporter <endpoint[,endpoint[,...]]> [<manifest>] <module_name> <block_height>",
	Short: "run substreams client periodically on a single block, exporting the values in prometheus format",
	Long: cli.Dedent(`
		Run substreams client periodically on a single block, exporting the values in prometheus format. The manifest is optional as it will try to find a file named 
		'substreams.yaml' in current working directory if nothing entered. You may enter a directory that contains a 'substreams.yaml' 
		file in place of '<manifest_file>'.
	`),
	RunE:         runPrometheus,
	Args:         cobra.RangeArgs(3, 4),
	SilenceUsage: true,
}

func init() {
	prometheusCmd.Flags().String("listen-addr", ":9102", "prometheus listen address")
	prometheusCmd.Flags().String("substreams-api-token-envvar", "SUBSTREAMS_API_TOKEN", "name of variable containing Substreams Authentication token")
	prometheusCmd.Flags().BoolP("insecure", "k", false, "Skip certificate validation on GRPC connection")
	prometheusCmd.Flags().BoolP("plaintext", "p", false, "Establish GRPC connection in plaintext")
	prometheusCmd.Flags().Duration("lookup_interval", time.Second*20, "endpoints will be polled at this interval")
	prometheusCmd.Flags().Duration("lookup_timeout", time.Second*10, "endpoints will be considered 'failing' if they don't complete in that duration")

	Cmd.AddCommand(prometheusCmd)
}

func runPrometheus(cmd *cobra.Command, args []string) error {

	endpoints := strings.Split(args[0], ",")
	manifestPathRaw := ""
	if len(args) == 4 {
		manifestPathRaw = args[1]
		args = args[1:]
	}
	manifestPath, err := ResolveManifestFile(manifestPathRaw)
	if err != nil {
		return fmt.Errorf("resolving manifest: %w", err)
	}
	moduleName := args[1]
	blockHeight := args[2]

	blockNum, err := strconv.ParseInt(blockHeight, 10, 64)
	addr := mustGetString(cmd, "listen-addr")

	manifestReader := manifest.NewReader(manifestPath)
	pkg, err := manifestReader.Read()
	if err != nil {
		return fmt.Errorf("reading manifest %q: %w", manifestPath, err)
	}

	outputStreamNames := strings.Split(moduleName, ",")

	apiToken := readAPIToken(cmd, "substreams-api-token-envvar")
	insecure := mustGetBool(cmd, "insecure")
	plaintext := mustGetBool(cmd, "plaintext")
	interval := mustGetDuration(cmd, "lookup_interval")
	timeout := mustGetDuration(cmd, "lookup_timeout")
	for _, endpoint := range endpoints {
		substreamsClientConfig := client.NewSubstreamsClientConfig(
			endpoint,
			apiToken,
			insecure,
			plaintext,
		)
		go launchSubstreamsPoller(endpoint, substreamsClientConfig, pkg.Modules, outputStreamNames, blockNum, interval, timeout)
	}

	promReg := prometheus.NewRegistry()
	promReg.MustRegister(status)
	promReg.MustRegister(requestDurationMs)

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
	status.With(prometheus.Labels{"endpoint": endpoint}).Set(1)
	requestDurationMs.With(prometheus.Labels{"endpoint": endpoint}).Set(float64(time.Since(begin).Milliseconds()))
}

func markFailure(endpoint string, begin time.Time) {
	status.With(prometheus.Labels{"endpoint": endpoint}).Set(0)
	requestDurationMs.With(prometheus.Labels{"endpoint": endpoint}).Set(float64(time.Since(begin).Milliseconds()))
}

func launchSubstreamsPoller(endpoint string, substreamsClientConfig *client.SubstreamsClientConfig, modules *pbsubstreams.Modules, outputStreamNames []string, blockNum int64, pollingInterval, pollingTimeout time.Duration) {
	sleep := time.Duration(0)
	for {
		time.Sleep(sleep)
		sleep = pollingInterval

		ctx, cancel := context.WithTimeout(context.Background(), pollingTimeout)
		begin := time.Now()
		ssClient, connClose, callOpts, err := client.NewSubstreamsClient(substreamsClientConfig)
		if err != nil {
			zlog.Error("substreams client setup", zap.Error(err))
			markFailure(endpoint, begin)
			cancel()
			continue
		}

		subReq := &pbsubstreams.Request{
			StartBlockNum: blockNum,
			StopBlockNum:  uint64(blockNum + 1),
			ForkSteps:     []pbsubstreams.ForkStep{pbsubstreams.ForkStep_STEP_IRREVERSIBLE},
			Modules:       modules,
			OutputModules: outputStreamNames,
		}

		if err := pbsubstreams.ValidateRequest(subReq, false); err != nil {
			zlog.Error("validate request", zap.Error(err))
			markFailure(endpoint, begin)
			connClose()
			cancel()
			continue
		}
		callOpts = append(callOpts, grpc.WaitForReady(false))
		cli, err := ssClient.Blocks(ctx, subReq, callOpts...)
		if err != nil {
			zlog.Error("call sf.substreams.v1.Stream/Blocks", zap.Error(err))
			markFailure(endpoint, begin)
			connClose()
			cancel()
			continue
		}

		var gotResp bool
		for {
			resp, err := cli.Recv()
			if resp != nil {
				switch resp.Message.(type) {
				case *pbsubstreams.Response_Data:
					fmt.Println(resp.Message.(*pbsubstreams.Response_Data).Data.Outputs)
					gotResp = true
				}
			}
			if err != nil {
				if err == io.EOF && gotResp {
					markSuccess(endpoint, begin)
				} else {
					zlog.Error("received error from substreams", zap.Error(err))
					markFailure(endpoint, begin)
				}
				break
			}
		}

		connClose()
		cancel()

	}
}

func readAPIToken(cmd *cobra.Command, envFlagName string) string {
	envVar := mustGetString(cmd, envFlagName)
	value := os.Getenv(envVar)
	if value != "" {
		return value
	}

	return os.Getenv("SF_API_TOKEN")
}
