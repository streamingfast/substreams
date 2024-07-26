package codegen

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/streamingfast/cli/sflags"
)

var SubgraphCmd = &cobra.Command{
	Use:   "subgraph [<manifest_url>] <module_name>",
	Short: "Generate subgraph dev environment from substreams manifest",
	Args:  cobra.RangeArgs(1, 2),
	RunE:  generateSubgraphEnv,
}

func generateSubgraphEnv(cmd *cobra.Command, args []string) error {
	manifestPath := ""
	moduleName := ""
	if len(args) == 2 {
		manifestPath = args[0]
		moduleName = args[1]
	}

	if len(args) == 1 {
		moduleName = args[0]
	}

	withDevEnv := sflags.MustGetBool(cmd, "with-dev-env")

	err := buildGenerateCommandFromArgs(manifestPath, moduleName, outputTypeSubgraph, withDevEnv)
	if err != nil {
		return fmt.Errorf("building generate command: %w", err)
	}

	return nil
}
