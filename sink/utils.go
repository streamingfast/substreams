package sink

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	networks "github.com/streamingfast/firehose-networks"
)

func readStartBlockFlag(cmd *cobra.Command, flagName string) (int64, bool, error) {
	val, err := cmd.Flags().GetString(flagName)
	if err != nil {
		panic(fmt.Sprintf("flags: couldn't find flag %q", flagName))
	}
	if val == "" {
		return 0, true, nil
	}

	startBlock, err := strconv.ParseInt(val, 10, 64)
	if err != nil {
		return 0, false, fmt.Errorf("start block is invalid: %w", err)
	}

	return startBlock, false, nil
}

func readStopBlockFlag(cmd *cobra.Command, startBlock int64, flagName string) (uint64, error) {
	val, err := cmd.Flags().GetString(flagName)
	if err != nil {
		panic(fmt.Sprintf("flags: couldn't find flag %q", flagName))
	}

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
