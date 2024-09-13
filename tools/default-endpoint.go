package tools

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/streamingfast/substreams/manifest"
)

var defaultEndpointCmd = &cobra.Command{
	Use:   "default-endpoint {network-name}",
	Short: "returns the default endpoint for a given network (the one used by default by the substreams CLI tool)",
	Args:  cobra.ExactArgs(1),
	RunE:  defaultEndpointE,
}

func init() {
	Cmd.AddCommand(defaultEndpointCmd)
}

func defaultEndpointE(cmd *cobra.Command, args []string) error {
	if endpoint := manifest.HardcodedEndpoints[args[0]]; endpoint != "" {
		fmt.Println(endpoint)
		return nil
	}
	return fmt.Errorf("no endpoint found for network %s", args[0])
}
