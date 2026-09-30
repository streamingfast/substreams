package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/streamingfast/cli"
	"github.com/streamingfast/cli/sflags"
	"github.com/streamingfast/derr"
	"github.com/streamingfast/substreams/sink"
	"github.com/streamingfast/substreams/sink/pubsub"
	"go.uber.org/zap"
)

func init() {
	sink.AddFlagsToSet(sinkPubsubCmd.Flags(),
		sink.FlagIncludeOptional(
			sink.FlagCursor,
			sink.FlagPartialBlocks,
		),
		sink.FlagExcludeDefault(
			sink.FlagDevelopmentMode,
			sink.FlagLiveBlockTimeDelta,
			sink.FlagMaxRetries,
		))

	sinkPubsubCmd.Flags().String("project", "", "Google Cloud project ID. Required unless <topic> is a projects/<project>/topics/<topic> path")
	sinkPubsubCmd.Flags().String("state-file", "./state.cursor", "File where the sink will store its cursor. A payload that could not be published is kept next to it in '<state-file>.pending'. If empty, no cursor will be saved or used, only the start-block, and a failed publish cannot be resumed.")
	sinkPubsubCmd.Flags().Int("pubsub-max-retries", 3, "Maximum number of retries for one publish (0 disables retries, -1 retries until the process is stopped)")
	sinkPubsubCmd.Flags().Duration("pubsub-timeout", 30*time.Second, "Timeout for one publish attempt")
	sinkPubsubCmd.Flags().Duration("pubsub-max-retry-interval", 30*time.Second, "Maximum interval between publish retries (exponential backoff cap)")
	sinkPubsubCmd.Flags().String("pubsub-on-failure", string(pubsub.OnFailureExit), fmt.Sprintf("What to do once every retry for a block has failed: %q (the default) keeps the block on disk, writes the reason to the termination log and exits with status %d; the next start publishes that block before it connects to Substreams. %q drops the block and continues (an undo notification is instead retried until it goes through)", pubsub.OnFailureExit, pubsub.ExitCodeDeliveryFailed, pubsub.OnFailureSkip))
	sinkPubsubCmd.Flags().Int("pubsub-batch-max-blocks", 0, "Publish up to this many blocks per message, in the webhook sink's batch payload (see below). 0 publishes one block per message in the single-block shape. Does not apply to a module that emits sf.substreams.sink.pubsub.v1.Publish")
	sinkPubsubCmd.Flags().Int("pubsub-batch-max-bytes", 0, "With --pubsub-batch-max-blocks, publish a batch before the next block would take its body past this many bytes. A block larger than this on its own is published alone. 0 means no limit")
	sinkPubsubCmd.Flags().Duration("pubsub-batch-max-wait", time.Second, "Longest a batch waits for more blocks before it is published, checked when the next block arrives. A batch is also published when the chain is live, before an undo notification, and when the stream ends")
	sinkPubsubCmd.Flags().Bool("pubsub-undo", false, "Publish a reorg notification on the same topic for each chain reorganization, with attribute type=undo and the webhook sink's undo body {\"lastValidBlock\": {\"number\": ..., \"id\": \"...\"}, \"manifest\": {\"moduleName\": \"...\"}}. Without it the cursor still moves back and the replacement blocks are published as usual. A module that emits sf.substreams.sink.pubsub.v1.Publish always publishes that format's reorg message, with or without this flag")
	sinkPubsubCmd.Flags().String("pubsub-termination-log", "/dev/termination-log", "File that receives the reason for a delivery-failure exit, written only when the file already exists (Kubernetes creates it)")

	SinkCmd.AddCommand(sinkPubsubCmd)
}

var sinkPubsubCmd = &cobra.Command{
	Use:   "pubsub <topic> [<manifest> [<module_name>]]",
	Short: "Publish the output of a substreams module to a Google Cloud Pub/Sub topic",
	Long: cli.Dedent(`
		Publish the output of a substreams module to a Google Cloud Pub/Sub topic. The block,
		batch, and undo bodies are the webhook sink's JSON ('substreams sink webhook').
		A module that emits sf.substreams.sink.pubsub.v1.Publish uses the
		substreams-sink-pubsub format described below.

		<topic> is a topic id, together with --project, or a full projects/<project>/topics/<topic>
		path. Credentials are application default credentials. When PUBSUB_EMULATOR_HOST is set,
		the sink publishes to that emulator.

		One message per block, attribute type=block:

		  {"clock": {"number": ..., "id": "...", "timestamp": "..."},
		   "manifest": {"moduleName": "...", "type": "..."},
		   "data": {...}}

		With --pubsub-batch-max-blocks=N every message carries up to N blocks, a batch of one
		included, and at most --pubsub-batch-max-bytes bytes when that is set, attribute type=batch:

		  {"manifest": {"moduleName": "...", "type": "..."},
		   "blocks": [{"clock": {...}, "data": {...}}, ...]}

		Message ordering is enabled. For the webhook JSON, every message uses the output module
		name as its ordering key, so a subscription created with message ordering enabled
		receives that module's messages in publish order. A failed publish resumes the key
		before the sink stops, so the next start is not stuck behind it.

		A module whose output is sf.substreams.sink.pubsub.v1.Publish is published in the
		substreams-sink-pubsub format. Each Publish.Message is one Pub/Sub message: its bytes
		and attributes are kept, attribute Cursor is set to the sink cursor, and the ordering
		key is the zero-padded block number and the message index. A reorg is a message with
		attributes LastValidBlock, Step=Undo, and Cursor, and no body. That message is published
		whether or not --pubsub-undo is set, and the batching flags do not apply.

		Once retries are exhausted the sink keeps the payload on disk and exits with status 75.
		--pubsub-on-failure=skip drops that block and continues instead. An undo notification
		is never dropped: it is retried until it is published, and no replacement block is
		published before then. That retry does not write a pending file.

		--pubsub-undo publishes a reorg notification on the same topic, attribute type=undo.
	`),
	RunE: sinkPubsubE,
	Args: cobra.RangeArgs(1, 3),
}

func sinkPubsubE(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	cmd.SilenceUsage = true

	topic := args[0]
	if _, _, err := pubsub.ParseTopic(sflags.MustGetString(cmd, "project"), topic); err != nil {
		return err
	}

	manifestPath, outputModule, err := ruiOrGuiManifestModulePositionalParams(args[1:])
	if err != nil {
		return err
	}

	sink.LoadSubstreamsAuthEnvFile(manifestPath)

	sinkerConfig, err := sink.ConfigFromViper(cmd, sink.IgnoreOutputModuleType, manifestPath, outputModule, "sink_pubsub", zlog, tracer)
	if err != nil {
		return err
	}

	onFailure, err := pubsub.ParseOnFailure(sflags.MustGetString(cmd, "pubsub-on-failure"))
	if err != nil {
		return fmt.Errorf("invalid --pubsub-on-failure: %w, expected one of %s", err, strings.Join(pubsub.OnFailureNames(), ", "))
	}

	maxRetries := sflags.MustGetInt(cmd, "pubsub-max-retries")
	if maxRetries < -1 {
		return fmt.Errorf("invalid --pubsub-max-retries %d, expected -1 or greater", maxRetries)
	}

	sinkConfig := pubsub.SinkConfig{
		Project:        sflags.MustGetString(cmd, "project"),
		Topic:          topic,
		PublishUndo:    sflags.MustGetBool(cmd, "pubsub-undo"),
		StateFile:      sflags.MustGetString(cmd, "state-file"),
		OnFailure:      onFailure,
		SinkerConfig:   sinkerConfig,
		BatchMaxBlocks: sflags.MustGetInt(cmd, "pubsub-batch-max-blocks"),
		BatchMaxBytes:  sflags.MustGetInt(cmd, "pubsub-batch-max-bytes"),
		BatchMaxWait:   sflags.MustGetDuration(cmd, "pubsub-batch-max-wait"),
		Retry: pubsub.RetryConfig{
			Timeout:     sflags.MustGetDuration(cmd, "pubsub-timeout"),
			MaxRetries:  maxRetries,
			MaxInterval: sflags.MustGetDuration(cmd, "pubsub-max-retry-interval"),
		},
		TerminationLogPath: sflags.MustGetString(cmd, "pubsub-termination-log"),
		Logger:             zlog,
	}

	pubsubSink, err := pubsub.NewSink(ctx, sinkConfig)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(ctx)
	go func() {
		<-derr.SetupSignalHandler(0)
		cancel()
	}()

	err = pubsubSink.Run(ctx)
	pubsubSink.PrintStats()
	if cerr := pubsubSink.Close(); cerr != nil {
		zlog.Warn("closing pubsub client", zap.Error(cerr))
	}

	var deliveryFailed *pubsub.DeliveryFailedError
	if errors.As(err, &deliveryFailed) {
		zlog.Error("stopping: pubsub delivery failed, the block is kept on disk and will be published first on the next start", zap.Error(err))
		os.Exit(pubsub.ExitCodeDeliveryFailed)
	}
	return err
}
