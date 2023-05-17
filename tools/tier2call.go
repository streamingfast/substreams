package tools

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/streamingfast/substreams/client"
	"github.com/streamingfast/substreams/manifest"
	pbssinternal "github.com/streamingfast/substreams/pb/sf/substreams/intern/v2"
)

var tier2CallCmd = &cobra.Command{
	Use:   "tier2call <manifest_url> <output_module> <start_block> <end_block>",
	Short: "Calls a tier2 service, for internal inspection",
	Args:  cobra.ExactArgs(4),
	RunE:  tier2CallE,
}

func init() {
	tier2CallCmd.Flags().String("substreams-api-token-envvar", "SUBSTREAMS_API_TOKEN", "name of variable containing Substreams Authentication token")
	tier2CallCmd.Flags().StringP("substreams-endpoint", "e", "mainnet.eth.streamingfast.io:443", "Substreams gRPC endpoint")
	tier2CallCmd.Flags().Bool("insecure", false, "Skip certificate validation on GRPC connection")
	tier2CallCmd.Flags().Bool("plaintext", false, "Establish GRPC connection in plaintext")

	tier2CallCmd.Flags().StringSliceP("params", "p", nil, "Set a params for parameterizable modules. Can be specified multiple times. Ex: -p module1=valA -p module2=valX&valY")

	Cmd.AddCommand(tier2CallCmd)
}

// delete all partial files which are already merged into the kv store
func tier2CallE(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	manifestPath := args[0]
	outputModule := args[1]
	startBlock, _ := strconv.ParseInt(args[2], 10, 64)
	stopBlock, _ := strconv.ParseInt(args[3], 10, 64)

	manifestReader, err := manifest.NewReader(manifestPath)
	if err != nil {
		return fmt.Errorf("manifest reader: %w", err)
	}

	pkg, err := manifestReader.Read()
	if err != nil {
		return fmt.Errorf("read manifest %q: %w", manifestPath, err)
	}

	if err := manifest.ApplyParams(mustGetStringSlice(cmd, "params"), pkg); err != nil {
		return fmt.Errorf("apply params: %w", err)
	}

	clientConfig := client.NewSubstreamsClientConfig(
		mustGetString(cmd, "substreams-endpoint"),
		ReadAPIToken(cmd, "substreams-api-token-envvar"),
		mustGetBool(cmd, "insecure"),
		mustGetBool(cmd, "plaintext"),
	)
	ssClient, _, callOpts, err := client.NewSubstreamsInternalClient(clientConfig)
	if err != nil {
		return fmt.Errorf("new internal client: %w", err)
	}

	req, err := ssClient.ProcessRange(ctx, &pbssinternal.ProcessRangeRequest{
		StartBlockNum: uint64(startBlock),
		StopBlockNum:  uint64(stopBlock),
		OutputModule:  outputModule,
		Modules:       pkg.Modules,
	}, callOpts...)
	if err != nil {
		return fmt.Errorf("process range request: %w", err)
	}

	for {
		msg, err := req.Recv()
		if err != nil {
			fmt.Println("Error: %w", err)
			break
		}
		cnt, _ := json.Marshal(msg)
		fmt.Println("Received message", string(cnt))
	}

	return nil
}
