package tools

import (
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"google.golang.org/grpc/metadata"

	"github.com/streamingfast/substreams/client"
	"github.com/streamingfast/substreams/manifest"
	pbssinternal "github.com/streamingfast/substreams/pb/sf/substreams/intern/v2"
)

var tier2CallCmd = &cobra.Command{
	Use:   "tier2call <manifest_url> <output_module> <stage_number> <start_block> <end_block>",
	Short: "Calls a tier2 service, for internal inspection",
	Args:  cobra.ExactArgs(5),
	RunE:  tier2CallE,
}

func init() {
	tier2CallCmd.Flags().String("substreams-api-token-envvar", "SUBSTREAMS_API_TOKEN", "name of variable containing Substreams Authentication token")
	tier2CallCmd.Flags().String("substreams-api-key-envvar", "SUBSTREAMS_API_KEY", "Name of variable containing Substreams Api Key")
	tier2CallCmd.Flags().StringP("substreams-endpoint", "e", "mainnet.eth.streamingfast.io:443", "Substreams gRPC endpoint")
	tier2CallCmd.Flags().Bool("insecure", false, "Skip certificate validation on GRPC connection")
	tier2CallCmd.Flags().Bool("plaintext", false, "Establish GRPC connection in plaintext")
	tier2CallCmd.Flags().StringSliceP("header", "H", nil, "Additional headers to be sent in the substreams request")
	tier2CallCmd.Flags().StringArrayP("params", "p", nil, "Set a params for parameterizable modules. Can be specified multiple times. Ex: -p module1=valA -p module2=valX&valY")
	tier2CallCmd.Flags().String("metering-plugin", "null://", "Metering configuration")
	tier2CallCmd.Flags().String("block-type", "sf.ethereum.type.v2.Block", "Block type")
	tier2CallCmd.Flags().String("merged-blocks-store-url", "./firehose-data/storage/merged-blocks", "Merged blocks store")
	tier2CallCmd.Flags().Uint64("state-bundle-size", uint64(1_000), "State segment size")
	tier2CallCmd.Flags().String("state-store-url", "./firehose-data/localdata", "Substreams state data storage")
	tier2CallCmd.Flags().String("state-store-default-tag", "", "Substreams state store default tag")

	Cmd.AddCommand(tier2CallCmd)
}

// delete all partial files which are already merged into the kv store
func tier2CallE(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	manifestPath := args[0]
	outputModule := args[1]
	stage, _ := strconv.ParseUint(args[2], 10, 32)
	startBlock, _ := strconv.ParseInt(args[3], 10, 64)
	stopBlock, _ := strconv.ParseInt(args[4], 10, 64)

	manifestReader, err := manifest.NewReader(manifestPath)
	if err != nil {
		return fmt.Errorf("manifest reader: %w", err)
	}

	pkg, _, err := manifestReader.Read()
	if err != nil {
		return fmt.Errorf("read manifest %q: %w", manifestPath, err)
	}

	params, err := manifest.ParseParams(mustGetStringArray(cmd, "params"))
	if err != nil {
		return fmt.Errorf("parsing params: %w", err)
	}

	if err := manifest.ApplyParams(params, pkg); err != nil {
		return fmt.Errorf("apply params: %w", err)
	}

	authToken, authType := GetAuth(cmd, "substreams-api-key-envvar", "substreams-api-token-envvar")
	clientConfig := client.NewSubstreamsClientConfig(
		mustGetString(cmd, "substreams-endpoint"),
		authToken,
		authType,
		mustGetBool(cmd, "insecure"),
		mustGetBool(cmd, "plaintext"),
	)
	ssClient, _, callOpts, headers, err := client.NewSubstreamsInternalClient(clientConfig)
	if err != nil {
		return fmt.Errorf("new internal client: %w", err)
	}

	// add auth header if available
	if headers.IsSet() {
		ctx = metadata.AppendToOutgoingContext(ctx, headers.ToArray()...)
	}
	//parse additional-headers flag
	additionalHeaders := mustGetStringSlice(cmd, "header")
	if additionalHeaders != nil {
		res := parseHeaders(additionalHeaders)
		headerArray := make([]string, 0, len(res)*2)
		for k, v := range res {
			headerArray = append(headerArray, k, v)
		}
		fmt.Println("the header array is this", headerArray)
		ctx = metadata.AppendToOutgoingContext(ctx, headerArray...)
	}

	meteringConfig := mustGetString(cmd, "metering-plugin")
	blockType := mustGetString(cmd, "block-type")
	stateStore := mustGetString(cmd, "state-store-url")
	stateStoreDefaultTag := mustGetString(cmd, "state-store-default-tag")
	mergedBlocksStore := mustGetString(cmd, "merged-blocks-store-url")
	stateBundleSize := mustGetUint64(cmd, "state-bundle-size")

	req, err := ssClient.ProcessRange(ctx, &pbssinternal.ProcessRangeRequest{
		StartBlockNum:        uint64(startBlock),
		StopBlockNum:         uint64(stopBlock),
		OutputModule:         outputModule,
		Modules:              pkg.Modules,
		Stage:                uint32(stage),
		MeteringConfig:       meteringConfig,
		BlockType:            blockType,
		MergedBlocksStore:    mergedBlocksStore,
		StateBundleSize:      stateBundleSize,
		StateStore:           stateStore,
		StateStoreDefaultTag: stateStoreDefaultTag,
	}, callOpts...)
	if err != nil {
		return fmt.Errorf("process range request: %w", err)
	}

	for {
		msg, err := req.Recv()
		if err != nil {
			fmt.Println("Error:", err)
			break
		}
		cnt, _ := json.Marshal(msg)
		fmt.Println("Received message", string(cnt))
	}

	return nil
}

// util to parse headers flags
func parseHeaders(headers []string) map[string]string {
	if headers == nil {
		return nil
	}
	result := make(map[string]string)
	for _, header := range headers {
		parts := strings.Split(header, ":")
		if len(parts) != 2 {
			log.Fatalf("invalid header format: %s", header)
		}
		result[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
	}
	return result
}
