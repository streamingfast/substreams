package sink

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	networks "github.com/streamingfast/firehose-networks"
)

func parseStartBlockFlag(val string) (int64, bool, error) {
	if val == "" {
		return 0, true, nil
	}

	startBlock, err := strconv.ParseInt(val, 10, 64)
	if err != nil {
		return 0, false, fmt.Errorf("start block is invalid: %w", err)
	}

	return startBlock, false, nil
}

func parseStopBlockFlag(val string, startBlock int64) (uint64, error) {
	// If empty, return 0 to indicate no stop block (infinite streaming)
	if val == "" {
		return 0, nil
	}

	isRelative := strings.HasPrefix(val, "+")
	if isRelative {
		if startBlock < 0 {
			return 0, fmt.Errorf("relative end block is supported only with an absolute start block")
		}

		val = strings.TrimPrefix(val, "+")
	}

	endBlock, err := strconv.ParseUint(val, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("end block is invalid: %w", err)
	}

	if isRelative {
		return uint64(startBlock) + endBlock, nil
	}

	return endBlock, nil
}

type BytesRepresentation int

const (
	BytesAsBase64 BytesRepresentation = iota
	BytesAsHex
	BytesAsString
	BytesAsBase58
)

// Helper to map string to BytesRepresentation
func BytesEncodingToRepresentation(enc string) BytesRepresentation {
	switch enc {
	case "base58":
		return BytesAsBase58
	case "base64":
		return BytesAsBase64
	case "string":
		return BytesAsString
	default:
		return BytesAsHex
	}
}

// InferBytesRepresentation infers the bytes representation based on the network or endpoint.
// It first checks the network ID, and if not found, it checks the endpoint.
// If neither is provided, it defaults to Hex encoding.
// It returns a dynamic.BytesRepresentation based on the encoding.
func InferBytesRepresentation(network string, endpoint string) BytesRepresentation {
	registry := networks.GetSubstreamsRegistry()

	// First check by network and aliases
	net := registry.Find(network)
	if net == nil {
		if endpoint == "" {
			return BytesAsHex
		}
		// Try with endpoint if no network was found
		net = registry.FindBySubstreamsEndpoint(endpoint)
	}

	// If network is found, try to extract bytes representation from it
	if net != nil {
		return BytesEncodingToRepresentation(string(networks.GetBytesEncoding(net)))
	}

	return BytesAsHex
}

// LoadSubstreamsAuthEnvFile loads authentication environment variables from .substreams.env file
func LoadSubstreamsAuthEnvFile(manifestPath string) {
	var projectPath string
	if manifestPath == "-" {
		// For stdin, use current working directory
		if wd, err := os.Getwd(); err == nil {
			projectPath = wd
		} else {
			projectPath = "."
		}
	} else {
		projectPath = filepath.Dir(manifestPath)
	}
	authFile := filepath.Join(projectPath, ".substreams.env")
	_, err := os.Stat(authFile)
	if err != nil {
		if os.IsNotExist(err) {
			authFile = ".substreams.env"
			_, err := os.Stat(authFile)
			if err != nil {
				if os.IsNotExist(err) {
					return
				} else {
					fmt.Printf("Error reading stats on auth file: %v: %s\n", authFile, err.Error())
					return
				}
			}
		} else {
			fmt.Printf("Error reading stats on auth file: %v: %s\n", authFile, err.Error())
			return
		}
	}

	cnt, err := os.ReadFile(authFile)
	if err != nil {
		fmt.Printf("Error reading auth file: %v: %s\n", authFile, err.Error())
		return
	}

	lines := strings.Split(string(cnt), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "export") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])
			fmt.Printf("Reading %s from %s\n", key, authFile)
			os.Setenv(key, value)
		}
	}
}
