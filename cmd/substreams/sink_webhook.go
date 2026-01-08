package main

import (
	"context"
	"time"

	"github.com/spf13/cobra"
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

	SinkCmd.AddCommand(sinkWebhookCmd)
}

// sinkWebhookCmd represents the command to run substreams webhook sink
var sinkWebhookCmd = &cobra.Command{
	Use:   "webhook <url> [<manifest> [<module_name>]]",
	Short: "Trigger a webhook call for each event from a substreams module",
	RunE:  sinkWebhookE,
	Args:  cobra.RangeArgs(1, 3),
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

	// Get webhook configuration from flags
	stateFileStr := sflags.MustGetString(cmd, "state-file")
	webhookTimeout := sflags.MustGetDuration(cmd, "webhook-timeout")
	webhookMaxRetries := sflags.MustGetInt(cmd, "webhook-max-retries")
	webhookMaxRetryInterval := sflags.MustGetDuration(cmd, "webhook-max-retry-interval")

	// Create webhook sink configuration
	sinkConfig := webhook.SinkConfig{
		WebhookURL:   webhookURL,
		StateFile:    stateFileStr,
		SinkerConfig: sinkerConfig,
		ClientConfig: webhook.Config{
			Timeout:     webhookTimeout,
			MaxRetries:  webhookMaxRetries,
			MaxInterval: webhookMaxRetryInterval,
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
