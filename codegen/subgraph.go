package codegen

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/streamingfast/cli/sflags"
)

var SubgraphCmd = &cobra.Command{
	Use:   "subgraph <manifest_url> <module_name> <network>",
	Short: "Generate subgraph dev environment from substreams manifest",
	Args:  cobra.ExactArgs(3),
	RunE:  generateSubgraphEnv,
}

func generateSubgraphEnv(cmd *cobra.Command, args []string) error {
	manifestPath := args[0]
	moduleName := args[1]
	networkName := args[2]
	withDevEnv := sflags.MustGetBool(cmd, "with-dev-env")

	err := buildGenerateCommandFromArgs(manifestPath, moduleName, networkName, outputTypeSubgraph, withDevEnv)
	if err != nil {
		return fmt.Errorf("building generate command: %w", err)
	}

	return nil
}
