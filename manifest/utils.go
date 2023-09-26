package manifest

import (
	"fmt"
	"os"
	"strings"

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

func ExtractNetworkEndpoint(networkFromManifest, fromFlag string) (string, error) {
	if fromFlag != "" {
		return fromFlag, nil
	}
	if networkFromManifest == "" {
		return "", fmt.Errorf("cannot determine endpoint. Either specify it with a flag, `-e mainnet.eth.streamingfast.io:443` or use the 'Network' field in the manifest, matching with SUBSTREAMS_ENDPOINTS_CONFIG_[network] environment variable")
	}

	endpoint := GetNetworkEndpointFromEnvironment(networkFromManifest)
	if endpoint == "" {
		return "", fmt.Errorf("cannot determine endpoint for network %q. Make sure that you set SUBSTREAMS_ENDPOINTS_CONFIG_%s environment variable to a valid endpoint", networkFromManifest, strings.ToUpper(networkFromManifest))
	}
	return endpoint, nil
}

func GetNetworkEndpointFromEnvironment(networkName string) string {
	networkEndpoint := os.Getenv(fmt.Sprintf("SUBSTREAMS_ENDPOINTS_CONFIG_%s", strings.ToUpper(networkName)))
	return networkEndpoint
}
