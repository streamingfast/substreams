package cli

import (
	"fmt"
	"github.com/streamingfast/bstream"
	"github.com/streamingfast/substreams/decode"
	"github.com/streamingfast/substreams/manifest"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"

	"github.com/spf13/cobra"
	"github.com/streamingfast/substreams/runtime"
)

func init() {
	remoteCmd.Flags().String("firehose-endpoint", "api.streamingfast.io:443", "firehose GRPC endpoint")
	remoteCmd.Flags().String("firehose-api-key-envvar", "FIREHOSE_API_KEY", "name of variable containing firehose authentication token (JWT)")
	remoteCmd.Flags().Int64P("start-block", "s", -1, "Start block for blockchain firehose")
	remoteCmd.Flags().Uint64P("stop-block", "t", 0, "Stop block for blockchain firehose")

	remoteCmd.Flags().BoolP("insecure", "k", false, "Skip certificate validation on GRPC connection")
	remoteCmd.Flags().BoolP("plaintext", "p", false, "Establish GRPC connection in plaintext")

	rootCmd.AddCommand(remoteCmd)
}

// remoteCmd represents the base command when called without any subcommands
var remoteCmd = &cobra.Command{
	Use:          "remote [manifest] [module_name]",
	Short:        "Run substreams remotely",
	RunE:         runRemote,
	Args:         cobra.ExactArgs(2),
	SilenceUsage: true,
}

func runRemote(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()

	config := &runtime.Config{
		ManifestPath:     args[0],
		OutputStreamName: args[1],
		StartBlock:       mustGetUint64(cmd, "start-block"),
		StopBlock:        mustGetUint64(cmd, "stop-block"),
		PrintMermaid:     true,
		RemoteConfig: &runtime.RemoteConfig{
			FirehoseEndpoint:     mustGetString(cmd, "firehose-endpoint"),
			FirehoseApiKeyEnvVar: mustGetString(cmd, "firehose-api-key-envvar"),
			InsecureMode:         mustGetBool(cmd, "insecure"),
			Plaintext:            mustGetBool(cmd, "plaintext"),
		},
	}

	manif, err := manifest.New(config.ManifestPath)
	if err != nil {
		return fmt.Errorf("read manifest %q: %w", config.ManifestPath, err)
	}

	config.ReturnHandler = decode.NewPrintReturnHandler(manif, config.OutputStreamName)
	if mustGetBool(cmd, "no-return-handler") {
		config.ReturnHandler = func(out *pbsubstreams.Output, step bstream.StepType, cursor *bstream.Cursor) error {
			return nil
		}
	}

	err = runtime.RemoteRun(ctx, config)
	if err != nil {
		return fmt.Errorf("running remote substream: %w", err)
	}

	return nil
}
