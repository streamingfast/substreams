package cli

import (
	"fmt"
	"strconv"

	"github.com/streamingfast/substreams/manifest"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/runtime"

	"github.com/spf13/cobra"
	"github.com/streamingfast/bstream"
	"github.com/streamingfast/substreams/decode"
)

var ProtobufBlockType string = "sf.ethereum.type.v1.Block"

func init() {
	localCmd.Flags().String("rpc-endpoint", "http://localhost:8546", "RPC endpoint of blockchain node")
	localCmd.Flags().StringArray("secondary-rpc-endpoints", nil, "RPC endpoints that will replace the primary in case of errors")
	localCmd.Flags().String("state-store-url", "./localdata", "URL of state store")
	localCmd.Flags().String("blocks-store-url", "./localblocks", "URL of blocks store")
	localCmd.Flags().String("rpc-cache-store-url", "./rpc-cache", "URL of blocks store")
	localCmd.Flags().String("irr-indexes-url", "./localirr", "URL of blocks store")
	localCmd.Flags().String("proto-url", "./proto", "Path of proto files")

	localCmd.Flags().Int64P("start-block", "s", -1, "Start block for blockchain firehose")
	localCmd.Flags().Uint64P("stop-block", "t", 0, "Stop block for blockchain firehose")
	localCmd.Flags().BoolP("partial", "p", false, "Produce partial stores")
	localCmd.Flags().Bool("no-return-handler", false, "Avoid printing output for module")
	localCmd.Flags().Bool("disable-database-transactions", false, "Disable transactions in database for faster inserts.")

	rootCmd.AddCommand(localCmd)
}

// localCmd represents the base command when called without any subcommands
var localCmd = &cobra.Command{
	Use:          "local [manifest] [module_name] [block_count]",
	Short:        "Run substreams locally",
	RunE:         runLocal,
	Args:         cobra.ExactArgs(3),
	SilenceUsage: true,
}

func runLocal(cmd *cobra.Command, args []string) error {
	if bstream.GetBlockDecoder == nil {
		return fmt.Errorf("cannot run local with a build that didn't include chain-specific decoders, compile from sf-ethereum or use the remote command")
	}

	// ISSUE A BIG WARNING IF WE HAVEN'T LOADED ALL THE CHAIN CONFIG SPECIFICS.
	// If we haven't compiled from `sf-ethereum`, we won't have the block readers, etc..

	cfg := &runtime.LocalConfig{
		BlocksStoreUrl:    mustGetString(cmd, "blocks-store-url"),
		IrrIndexesUrl:     mustGetString(cmd, "irr-indexes-url"),
		StateStoreUrl:     mustGetString(cmd, "state-store-url"),
		RpcEndpoint:       mustGetString(cmd, "rpc-endpoint"),
		SecondaryRpcEndpoints: mustGetStringArray(cmd, "secondary-rpc-endpoints"),RpcCacheUrl:       mustGetString(cmd, "rpc-cache-store-url"),
		PartialMode:       mustGetBool(cmd, "partial"),
		ProtoUrl:          mustGetString(cmd, "proto-url"),
		ProtobufBlockType: ProtobufBlockType,
		Config: &runtime.Config{
			ManifestPath:     args[0],
			OutputStreamName: args[1],
			StartBlock:       uint64(mustGetInt64(cmd, "start-block")),
			StopBlock:        mustGetUint64(cmd, "stop-block"),
			PrintMermaid:     true,
		},
	}

	if cfg.StopBlock == 0 {
		var blockCount uint64 = 1000
		if len(args) > 0 {
			val, err := strconv.ParseInt(args[2], 10, 64)
			if err != nil {
				return fmt.Errorf("invalid block count %s", args[2])
			}
			blockCount = uint64(val)
		}

		cfg.StopBlock = cfg.StartBlock + blockCount
	}

	manif, err := manifest.New(cfg.ManifestPath)
	if err != nil {
		return fmt.Errorf("read manifest %q: %w", cfg.ManifestPath, err)
	}

	cfg.ReturnHandler = decode.NewPrintReturnHandler(manif, cfg.OutputStreamName)
	if mustGetBool(cmd, "no-return-handler") {
		cfg.ReturnHandler = func(out *pbsubstreams.Output, step bstream.StepType, cursor *bstream.Cursor) error {
			return nil
		}
	}

	err = runtime.LocalRun(cmd.Context(), cfg)
	if err != nil {
		return fmt.Errorf("running local substream: %w", err)
	}

	return nil
}
