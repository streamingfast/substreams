package manifest

import (
	"fmt"
	"os"
	"strings"

	networks "github.com/streamingfast/firehose-networks"
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

	endpoint := networks.GetSubstreamsEndpoint(networkFromManifest)
	if endpoint != "" {
		logger.Info("using endpoint from registry", zap.String("manifest_network", networkFromManifest), zap.String("endpoint", endpoint))
		return endpoint, nil
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
