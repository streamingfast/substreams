package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/streamingfast/cli"
	"github.com/streamingfast/cli/sflags"
	"github.com/streamingfast/derr"
	"github.com/streamingfast/substreams/sink"
	"github.com/streamingfast/substreams/sink/webhook"
)

func init() {
	// default sinker flags
	sink.AddFlagsToSet(sinkWebhookCmd.Flags(),
		sink.FlagIncludeOptional(
			sink.FlagCursor,
			sink.FlagPartialBlocks,
		),
		sink.FlagExcludeDefault(
			sink.FlagDevelopmentMode,
			sink.FlagLiveBlockTimeDelta,
			sink.FlagMaxRetries,
		))

	sinkWebhookCmd.Flags().String("state-file", "./state.cursor", "File where the sink will store its cursor. If empty, no cursor will be saved or used, only the start-block.")
	sinkWebhookCmd.Flags().Int("webhook-max-retries", 3, "Maximum number of retries for webhook calls (0 disables retries, -1 for infinite retries)")
	sinkWebhookCmd.Flags().Duration("webhook-timeout", 30*time.Second, "Timeout for individual webhook calls")
	sinkWebhookCmd.Flags().Duration("webhook-max-retry-interval", 30*time.Second, "Maximum interval between webhook retries (exponential backoff cap)")
	sinkWebhookCmd.Flags().String("webhook-auth-header-name", webhook.DefaultAuthHeaderName, "Name of the header carrying the value read from --webhook-auth-header-value-envvar")
	sinkWebhookCmd.Flags().String("webhook-auth-header-value-envvar", "WEBHOOK_AUTH_HEADER_VALUE", "Environment variable holding the auth header value sent on every call, for example 'Bearer <token>'. No header is sent when the variable is empty")
	sinkWebhookCmd.Flags().String("webhook-signing-secret-envvar", "WEBHOOK_SIGNING_SECRET", fmt.Sprintf("Environment variable holding the secret used to sign every call body with HMAC-SHA256 in the %s header. No signature is sent when the variable is empty", webhook.SignatureHeader))

	SinkCmd.AddCommand(sinkWebhookCmd)
}

// sinkWebhookCmd represents the command to run substreams webhook sink
var sinkWebhookCmd = &cobra.Command{
	Use:   "webhook <url> [<manifest> [<module_name>]]",
	Short: "Trigger a webhook call for each event from a substreams module",
	Long: cli.Dedent(`
		POST the output of a substreams module to <url>, one call per block, as JSON:

		  {"clock": {"number": ..., "id": "...", "timestamp": "..."},
		   "manifest": {"moduleName": "...", "type": "..."},
		   "data": {...}}

		Calls carry the header named by --webhook-auth-header-name when the variable named by
		--webhook-auth-header-value-envvar is set, and an ` + webhook.SignatureHeader + ` header when the
		variable named by --webhook-signing-secret-envvar is set. The signature value is
		"t=<unix seconds>,v1=<hex>" where <hex> is HMAC-SHA256 over "<t>.<body>" keyed with the secret.

		Network errors and 5xx responses are retried with exponential backoff up to --webhook-max-retries.
		A 4xx response is not retried.
	`),
	RunE: sinkWebhookE,
	Args: cobra.RangeArgs(1, 3),
}

func sinkWebhookE(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	cmd.SilenceUsage = true

	webhookURL := args[0]

	manifestPath, outputModule, err := ruiOrGuiManifestModulePositionalParams(args[1:])
	if err != nil {
		return err
	}

	// Load auth environment file if it exists
	sink.LoadSubstreamsAuthEnvFile(manifestPath)

	// parses flags
	sinkerConfig, err := sink.ConfigFromViper(cmd, sink.IgnoreOutputModuleType, manifestPath, outputModule, "sink_webhook", zlog, tracer)
	if err != nil {
		return err
	}

	sinkConfig := webhook.SinkConfig{
		WebhookURL:   webhookURL,
		StateFile:    sflags.MustGetString(cmd, "state-file"),
		SinkerConfig: sinkerConfig,
		ClientConfig: webhook.Config{
			Timeout:         sflags.MustGetDuration(cmd, "webhook-timeout"),
			MaxRetries:      sflags.MustGetInt(cmd, "webhook-max-retries"),
			MaxInterval:     sflags.MustGetDuration(cmd, "webhook-max-retry-interval"),
			AuthHeaderName:  sflags.MustGetString(cmd, "webhook-auth-header-name"),
			AuthHeaderValue: os.Getenv(sflags.MustGetString(cmd, "webhook-auth-header-value-envvar")),
			SigningSecret:   os.Getenv(sflags.MustGetString(cmd, "webhook-signing-secret-envvar")),
		},
		Logger: zlog,
	}

	// Create and run the webhook sink
	webhookSink, err := webhook.NewSink(sinkConfig)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(ctx)
	go func() {
		<-derr.SetupSignalHandler(0)
		cancel()
	}()

	err = webhookSink.Run(ctx)

	// Print final statistics
	webhookSink.PrintStats()

	return err
}
