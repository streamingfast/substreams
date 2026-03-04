package tools

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"google.golang.org/grpc/metadata"

	"github.com/streamingfast/cli/sflags"
	"github.com/streamingfast/substreams/client"
	"github.com/streamingfast/substreams/manifest"
	pbssinternal "github.com/streamingfast/substreams/pb/sf/substreams/intern/v2"
)

var tier2CallCmd = &cobra.Command{
	Use:   "tier2call <manifest_url> <output_module> <stage_number> <segment_number>",
	Short: "Calls a tier2 service, for internal inspection",
	Args:  cobra.ExactArgs(4),
	RunE:  tier2CallE,
}

func init() {
	tier2CallCmd.Flags().String("substreams-api-token-envvar", "SUBSTREAMS_API_TOKEN", "name of variable containing Substreams Authentication token")
	tier2CallCmd.Flags().String("substreams-api-key-envvar", "SUBSTREAMS_API_KEY", "Name of variable containing Substreams Api Key")
	tier2CallCmd.Flags().StringP("substreams-endpoint", "e", "mainnet.eth.streamingfast.io:443", "Substreams gRPC endpoint")
	tier2CallCmd.Flags().Bool("insecure", false, "Skip certificate validation on GRPC connection")
	tier2CallCmd.Flags().Bool("plaintext", false, "Establish GRPC connection in plaintext")
	tier2CallCmd.Flags().StringSliceP("header", "H", nil, "Additional headers to be sent in the substreams request. Common headers: X-Substreams-Parallel-Workers, X-substreams-acknowledge-non-deterministic. Operators/developers can also pass environment-specific headers. For more details, see https://docs.substreams.dev/reference-material/command-line-interface#headers")
	tier2CallCmd.Flags().StringArrayP("params", "p", nil, "Set a params for parameterizable modules. Can be specified multiple times. Ex: -p module1=valA -p module2=valX&valY")
	tier2CallCmd.Flags().String("metering-plugin", "null://", "Metering configuration")
	tier2CallCmd.Flags().String("block-type", "sf.ethereum.type.v2.Block", "Block type")
	tier2CallCmd.Flags().String("foundational-stores-config-path", "", "path to file containing foundational stores configuration")
	tier2CallCmd.Flags().String("merged-blocks-store-url", "./firehose-data/storage/merged-blocks", "Merged blocks store")
	tier2CallCmd.Flags().Uint64("state-bundle-size", uint64(1_000), "State segment size")
	tier2CallCmd.Flags().Uint64("first-streamable-block", 0, "First Streamable block on the chain (usually 0 or 1, some networks start at arbitrary block numbers)")
	tier2CallCmd.Flags().String("state-store-url", "./firehose-data/localdata", "Substreams state data storage")
	tier2CallCmd.Flags().String("state-store-default-tag", "", "Substreams state store default tag")
	tier2CallCmd.Flags().String("extension-configs", "", "semicolon-separted list of k=v values. Ex: rpc_eth_call=50000000,http://localhost:8000;a=b;c=d")
	tier2CallCmd.Flags().Bool("skip-package-validation", false, "Do not perform any validation when reading substreams package")

	Cmd.AddCommand(tier2CallCmd)
}

func tier2CallE(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	manifestPath := args[0]
	outputModule := args[1]
	stage, _ := strconv.ParseUint(args[2], 10, 32)
	segmentNumber, _ := strconv.ParseUint(args[3], 10, 32)

	options := []manifest.Option{}

	if sflags.MustGetBool(cmd, "skip-package-validation") {
		options = append(options, manifest.SkipPackageValidationReader())
	}

	manifestReader, err := manifest.NewReader(manifestPath, options...)
	if err != nil {
		return fmt.Errorf("manifest reader: %w", err)
	}

	pkgBundle, err := manifestReader.Read()
	if err != nil {
		return fmt.Errorf("read manifest %q: %w", manifestPath, err)
	}

	if pkgBundle == nil {
		return fmt.Errorf("no package found")
	}

	pkg := pkgBundle.Package
	params, err := manifest.ParseParams(sflags.MustGetStringArray(cmd, "params"))
	if err != nil {
		return fmt.Errorf("parsing params: %w", err)
	}

	if err := manifest.ApplyParams(params, pkg); err != nil {
		return fmt.Errorf("apply params: %w", err)
	}

	authToken, authType := GetAuth(cmd, "substreams-api-key-envvar", "substreams-api-token-envvar")
	clientConfig := client.NewSubstreamsClientConfig(client.SubstreamsClientConfigOptions{
		Endpoint:             sflags.MustGetString(cmd, "substreams-endpoint"),
		AuthToken:            authToken,
		AuthType:             authType,
		Insecure:             sflags.MustGetBool(cmd, "insecure"),
		PlainText:            sflags.MustGetBool(cmd, "plaintext"),
		Agent:                "substreams_tools",
		ForceProtocolVersion: client.ProtocolVersionUnset, // irrelevant for tier2 calls
	})
	ssClient, _, callOpts, headers, err := client.NewSubstreamsInternalClient(clientConfig)
	if err != nil {
		return fmt.Errorf("new internal client: %w", err)
	}

	// add auth header if available
	if headers.IsSet() {
		ctx = metadata.AppendToOutgoingContext(ctx, headers.ToArray()...)
	}
	//parse additional-headers flag
	additionalHeaders := sflags.MustGetStringSlice(cmd, "header")
	if len(additionalHeaders) != 0 {
		res := parseHeaders(additionalHeaders)
		headerArray := make([]string, 0, len(res)*2)
		for k, v := range res {
			headerArray = append(headerArray, k, v)
		}
		ctx = metadata.AppendToOutgoingContext(ctx, headerArray...)
	}

	meteringConfig := sflags.MustGetString(cmd, "metering-plugin")
	blockType := sflags.MustGetString(cmd, "block-type")
	stateStore := sflags.MustGetString(cmd, "state-store-url")
	stateStoreDefaultTag := sflags.MustGetString(cmd, "state-store-default-tag")
	mergedBlocksStore := sflags.MustGetString(cmd, "merged-blocks-store-url")
	stateBundleSize := sflags.MustGetUint64(cmd, "state-bundle-size")
	firstStreamableBlock := sflags.MustGetUint64(cmd, "first-streamable-block")

	foundationalStoreConfigPath := sflags.MustGetString(cmd, "foundational-stores-config-path")
	foundationStoreConfigs, err := loadTier1FoundationalStoreEndpoints(foundationalStoreConfigPath)
	if err != nil {
		fmt.Println("foundational store config path error: ", err)
	}

	var wasmExtensionConfigs map[string]string
	extensionConfigs := sflags.MustGetString(cmd, "extension-configs")
	if extensionConfigs != "" {
		wasmExtensionConfigs = make(map[string]string)
		for _, config := range strings.Split(extensionConfigs, ";") {
			if config == "" {
				continue
			}
			parts := strings.SplitN(config, "=", 2)
			if len(parts) != 2 {
				return fmt.Errorf("invalid extension config: %s", config)
			}
			wasmExtensionConfigs[parts[0]] = parts[1]
		}
	}

	req, err := ssClient.ProcessRange(ctx, &pbssinternal.ProcessRangeRequest{
		SegmentSize:                stateBundleSize,
		SegmentNumber:              segmentNumber,
		OutputModule:               outputModule,
		Modules:                    pkg.Modules,
		Stage:                      uint32(stage),
		MeteringConfig:             meteringConfig,
		BlockType:                  blockType,
		FirstStreamableBlock:       firstStreamableBlock,
		MergedBlocksStore:          mergedBlocksStore,
		StateStore:                 stateStore,
		StateStoreDefaultTag:       stateStoreDefaultTag,
		WasmExtensionConfigs:       wasmExtensionConfigs,
		FoundationalStoreEndpoints: foundationStoreConfigs,
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

func loadTier1FoundationalStoreEndpoints(configPath string) (map[string]string, error) {
	if configPath == "" {
		return nil, nil
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read foundational stores config file %s: %w", configPath, err)
	}

	var endpoints map[string]string
	if err := json.Unmarshal(data, &endpoints); err != nil {
		return nil, fmt.Errorf("failed to parse foundational stores config file %s: %w", configPath, err)
	}

	return endpoints, nil
}
