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

// TODO: replace by the blockchain-based discovery when available
var HardcodedEndpoints = map[string]string{
	"mainnet":        "mainnet.eth.streamingfast.io:443",
	"matic":          "polygon.streamingfast.io:443",
	"polygon":        "polygon.streamingfast.io:443",
	"goerli":         "goerli.eth.streamingfast.io:443",
	"mumbai":         "mumbai.streamingfast.io:443",
	"bnb":            "bnb.streamingfast.io:443",
	"bsc":            "bnb.streamingfast.io:443",
	"sepolia":        "sepolia.eth.streamingfast.io:443",
	"holesky":        "holesky.eth.streamingfast.io:443",
	"near":           "mainnet.near.streamingfast.io:443",
	"near-mainnet":   "mainnet.near.streamingfast.io:443",
	"near-testnet":   "testnet.near.streamingfast.io:443",
	"arbitrum":       "arb-one.streamingfast.io:443",
	"arb":            "arb-one.streamingfast.io:443",
	"arb-one":        "arb-one.streamingfast.io:443",
	"solana":         "mainnet.sol.streamingfast.io:443",
	"sol":            "mainnet.sol.streamingfast.io:443",
	"solana-mainnet": "mainnet.sol.streamingfast.io:443",

	//"antelope": "",
}
