package manifest

import (
	"fmt"
	"os"
	"strings"

	"go.uber.org/zap"

	"gopkg.in/yaml.v3"
)

// mapSlice represents a map in the form of a list of key/value pairs (key/value
// pair of `[2]string` where index 0 is the key and index 1 is the value).
type mapSlice [][2]string

func (s mapSlice) MarshalYAML() (interface{}, error) {
	m := map[string]string{}
	for _, kv := range s {
		m[kv[0]] = kv[1]
	}

	return m, nil
}

func (s *mapSlice) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind != yaml.MappingNode {
		return fmt.Errorf("expected map")
	}

	if len(n.Content)%2 != 0 {
		return fmt.Errorf("invalid map, unequal number of nodes below")
	}

	for i := 0; i < len(n.Content); i += 2 {
		k := n.Content[i].Value
		v := n.Content[i+1].Value
		*s = append(*s, [2]string{k, v})
	}

	return nil
}

func ExtractNetworkEndpoint(networkFromManifest, fromFlag string, logger *zap.Logger) (string, error) {
	if fromFlag != "" {
		return fromFlag, nil
	}

	if networkFromManifest == "" {
		logger.Warn("DEPRECATION WARNING: This substreams does not define a 'network' field. To allow endpoint inference, define a 'network' field in your Substreams manifest. See --help for more information. Assuming 'mainnet' as network")
		networkFromManifest = "mainnet"
	}

	if endpoint := getNetworkEndpointFromEnvironment(networkFromManifest); endpoint != "" {
		logger.Info("using endpoint from environment", zap.String("manifest_network", networkFromManifest), zap.String("endpoint", endpoint))
		return endpoint, nil
	}

	if ep, ok := HardcodedEndpoints[networkFromManifest]; ok {
		logger.Info("using endpoint from hardcoded list", zap.String("manifest_network", networkFromManifest), zap.String("endpoint", ep))
		return ep, nil
	}

	return "", fmt.Errorf("cannot determine endpoint for network %q: make sure that you set SUBSTREAMS_ENDPOINTS_CONFIG_%s environment variable to a valid endpoint, or use the endpoint flag", networkFromManifest, strings.ToUpper(networkFromManifest))
}

func getNetworkEndpointFromEnvironment(networkName string) string {
	networkEndpoint := os.Getenv(fmt.Sprintf("SUBSTREAMS_ENDPOINTS_CONFIG_%s", strings.ToUpper(networkName)))
	return networkEndpoint
}

func searchExistingCaseInsensitiveFileName(dir, filename string) (string, error) {
	files, err := os.ReadDir(dir)
	if err != nil {
		return "", fmt.Errorf("reading dir: %w", err)
	}

	for _, file := range files {
		if strings.EqualFold(file.Name(), filename) {
			return file.Name(), nil
		}
	}
	return "", os.ErrNotExist
}

// TODO: replace by the blockchain-based discovery when available
var HardcodedEndpoints = map[string]string{
	"mainnet":             "mainnet.eth.streamingfast.io:443",
	"matic":               "polygon.streamingfast.io:443",
	"polygon":             "polygon.streamingfast.io:443",
	"amoy":                "amoy.substreams.pinax.network:443",
	"polygon-amoy":        "amoy.substreams.pinax.network:443",
	"goerli":              "goerli.eth.streamingfast.io:443",
	"mumbai":              "mumbai.streamingfast.io:443",
	"bnb":                 "bnb.streamingfast.io:443",
	"bsc":                 "bnb.streamingfast.io:443",
	"base":                "base-mainnet.streamingfast.io:443",
	"sepolia":             "sepolia.eth.streamingfast.io:443",
	"holesky":             "holesky.eth.streamingfast.io:443",
	"near":                "mainnet.near.streamingfast.io:443",
	"near-mainnet":        "mainnet.near.streamingfast.io:443",
	"arbitrum":            "arb-one.streamingfast.io:443",
	"arb":                 "arb-one.streamingfast.io:443",
	"arb-one":             "arb-one.streamingfast.io:443",
	"arbitrum-one":        "arb-one.streamingfast.io:443",
	"solana":              "mainnet.sol.streamingfast.io:443",
	"sol":                 "mainnet.sol.streamingfast.io:443",
	"solana-mainnet":      "mainnet.sol.streamingfast.io:443",
	"solana-mainnet-beta": "mainnet.sol.streamingfast.io:443",
	"optimism":            "optimism.streamingfast.io:443",
	"bitcoin":             "btc-mainnet.streamingfast.io:443",
	"chapel":              "chapel.substreams.pinax.network:443",
	"injective-mainnet":   "mainnet.injective.streamingfast.io:443",
	"injective-testnet":   "testnet.injective.streamingfast.io:443",
	"sei":                 "evm-mainnet.sei.streamingfast.io:443",
	"sei-mainnet":         "evm-mainnet.sei.streamingfast.io:443",
	"sei-evm-mainnet":     "evm-mainnet.sei.streamingfast.io:443",
	"starknet-mainnet":    "mainnet.starknet.streamingfast.io:443",
	"starknet":            "mainnet.starknet.streamingfast.io:443",
	"starknet-testnet":    "testnet.starknet.streamingfast.io:443",
	"vara-mainnet":        "mainnet.vara.streamingfast.io:443",
	"vara-testnet":        "testnet.vara.streamingfast.io:443",

	// antelope chains
	"eos":       "eos.substreams.pinax.network:443",
	"jungle4":   "jungle4.substreams.pinax.network:443",
	"kylin":     "kylin.substreams.pinax.network:443",
	"wax":       "wax.substreams.pinax.network:443",
	"waxtest":   "waxtest.substreams.pinax.network:443",
	"telos":     "telos.substreams.pinax.network:443",
	"telostest": "telostest.substreams.pinax.network:443",
	"ore":       "ore.substreams.pinax.network:443",
	"orestage":  "orestage.substreams.pinax.network:443",
	"ux":        "ux.substreams.pinax.network:443",
}
