package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/streamingfast/cli"
	"github.com/streamingfast/cli/sflags"
	"github.com/streamingfast/derr"
	"github.com/streamingfast/substreams/sink"
	"github.com/streamingfast/substreams/sink/webhook"
	"go.uber.org/zap"
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

	sinkWebhookCmd.Flags().String("state-file", "./state.cursor", "File where the sink will store its cursor, a payload that could not be delivered is kept next to it in '<state-file>.pending'. If empty, no cursor will be saved or used, only the start-block, and a failed delivery cannot be resumed.")
	sinkWebhookCmd.Flags().Int("webhook-max-retries", 3, "Maximum number of retries for webhook calls (0 disables retries, -1 for infinite retries)")
	sinkWebhookCmd.Flags().Duration("webhook-timeout", 30*time.Second, "Timeout for individual webhook calls")
	sinkWebhookCmd.Flags().Duration("webhook-max-retry-interval", 30*time.Second, "Maximum interval between webhook retries (exponential backoff cap)")
	sinkWebhookCmd.Flags().String("webhook-on-failure", string(webhook.OnFailureSkip), fmt.Sprintf("What to do once every retry for a block has failed: %q drops the block and continues, %q keeps the block on disk, writes the reason to the termination log and exits with status %d; the next start delivers that block before it connects to Substreams", webhook.OnFailureSkip, webhook.OnFailureExit, webhook.ExitCodeDeliveryFailed))
	sinkWebhookCmd.Flags().Int("webhook-batch-max-blocks", 0, "Send up to this many blocks per call, in the batch payload shape (see below). 0 sends one block per call in the single-block shape")
	sinkWebhookCmd.Flags().Duration("webhook-batch-max-wait", time.Second, "Longest a batch waits for more blocks before it is sent, checked when the next block arrives. A batch is also sent when the chain is live, before an undo notification, and when the stream ends")
	sinkWebhookCmd.Flags().String("webhook-undo-url", "", "URL that receives a POST for each chain reorganization, with body {\"lastValidBlock\": {\"number\": ..., \"id\": \"...\"}, \"manifest\": {\"moduleName\": \"...\"}}. Empty disables the notification; the blocks that replace the undone ones are still delivered to <url>")
	sinkWebhookCmd.Flags().String("webhook-termination-log", "/dev/termination-log", "File that receives the reason for a delivery-failure exit, written only when the file already exists (Kubernetes creates it)")
	sinkWebhookCmd.Flags().String("webhook-auth-header-name", webhook.DefaultAuthHeaderName, "Name of the header carrying the value read from --webhook-auth-header-value-envvar")
	sinkWebhookCmd.Flags().String("webhook-auth-header-value-envvar", "WEBHOOK_AUTH_HEADER_VALUE", "Environment variable holding the auth header value sent on every call, for example 'Bearer <token>'. No header is sent when the variable is empty")
	sinkWebhookCmd.Flags().String("webhook-signing-secret-envvar", "WEBHOOK_SIGNING_SECRET", fmt.Sprintf("Environment variable holding the secret used to sign every call body with HMAC-SHA256 in the %s header. No signature is sent when the variable is empty", webhook.SignatureHeader))

	SinkCmd.AddCommand(sinkWebhookCmd)
}

// sinkWebhookCmd represents the command to run substreams webhook sink
var sinkWebhookCmd = &cobra.Command{
	Use:   "webhook <url> [<manifest> [<module_name>]]",
	Short: "POST the output of a substreams module to a webhook, one call per block or per batch of blocks",
	Long: cli.Dedent(`
		POST the output of a substreams module to <url>, one call per block, as JSON:

		  {"clock": {"number": ..., "id": "...", "timestamp": "..."},
		   "manifest": {"moduleName": "...", "type": "..."},
		   "data": {...}}

		With --webhook-batch-max-blocks=N every call carries up to N blocks, a batch of one included, as:

		  {"manifest": {"moduleName": "...", "type": "..."},
		   "blocks": [{"clock": {...}, "data": {...}}, ...]}

		Blocks are in ascending order. Switching batching on or off while the sink is stopped discards a
		pending payload of the other shape; its blocks come back through the stream in the new shape.

		Calls carry the header named by --webhook-auth-header-name when the variable named by
		--webhook-auth-header-value-envvar is set, and an ` + webhook.SignatureHeader + ` header when the
		variable named by --webhook-signing-secret-envvar is set. The signature value is
		"t=<unix seconds>,v1=<hex>" where <hex> is HMAC-SHA256 over "<t>.<body>" keyed with the secret.

		Network errors and 5xx responses are retried with exponential backoff up to --webhook-max-retries.
		A 4xx response is not retried. --webhook-on-failure decides what happens after the last retry.

		With --webhook-on-failure=exit the payload that could not be delivered is written to
		'<state-file>.pending', and the next start delivers it before it opens a Substreams stream. The headers
		are recomputed from the current environment, so a rotated secret or a new URL applies to the retry. A
		kill in the middle of a call leaves no pending file: the cursor was not saved, so the stream re-sends
		that block.

		Reorganizations: with --final-blocks-only the sink only delivers blocks past finality and never sees
		one. With --undo-buffer-size=N it holds the last N blocks back and absorbs any reorganization that fits
		in them. Otherwise, or for a reorganization deeper than the buffer, the receiver has already been sent
		blocks that are no longer on the chain: --webhook-undo-url receives a notification naming the last valid
		block, then the replacement blocks arrive as regular calls. Undo notifications follow the same retry,
		on-failure and pending-file rules as blocks.
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

	onFailure, err := webhook.ParseOnFailure(sflags.MustGetString(cmd, "webhook-on-failure"))
	if err != nil {
		return err
	}

	sinkConfig := webhook.SinkConfig{
		WebhookURL:     webhookURL,
		UndoURL:        sflags.MustGetString(cmd, "webhook-undo-url"),
		StateFile:      sflags.MustGetString(cmd, "state-file"),
		OnFailure:      onFailure,
		SinkerConfig:   sinkerConfig,
		BatchMaxBlocks: sflags.MustGetInt(cmd, "webhook-batch-max-blocks"),
		BatchMaxWait:   sflags.MustGetDuration(cmd, "webhook-batch-max-wait"),
		ClientConfig: webhook.Config{
			Timeout:         sflags.MustGetDuration(cmd, "webhook-timeout"),
			MaxRetries:      sflags.MustGetInt(cmd, "webhook-max-retries"),
			MaxInterval:     sflags.MustGetDuration(cmd, "webhook-max-retry-interval"),
			AuthHeaderName:  sflags.MustGetString(cmd, "webhook-auth-header-name"),
			AuthHeaderValue: os.Getenv(sflags.MustGetString(cmd, "webhook-auth-header-value-envvar")),
			SigningSecret:   os.Getenv(sflags.MustGetString(cmd, "webhook-signing-secret-envvar")),
		},
		TerminationLogPath: sflags.MustGetString(cmd, "webhook-termination-log"),
		Logger:             zlog,
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

	var deliveryFailed *webhook.DeliveryFailedError
	if errors.As(err, &deliveryFailed) {
		zlog.Error("stopping: webhook delivery failed, the block is kept on disk and will be delivered first on the next start", zap.Error(err))
		os.Exit(webhook.ExitCodeDeliveryFailed)
	}

	return err
}
