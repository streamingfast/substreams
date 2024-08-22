package codegen

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/streamingfast/cli/sflags"
)

var SubgraphCmd = &cobra.Command{
	Use:   "subgraph [<manifest_url>]",
	Short: "Generate subgraph dev environment from substreams manifest",
	Args:  cobra.RangeArgs(0, 1),
	RunE:  generateSubgraphEnv,
}

func generateSubgraphEnv(cmd *cobra.Command, args []string) error {
	manifestPath := ""
	if len(args) == 1 {
		manifestPath = args[0]
	}

	withDevEnv := sflags.MustGetBool(cmd, "with-dev-env")

	err := buildGenerateCommandFromArgs(manifestPath, Subgraph, withDevEnv)
	if err != nil {
		return fmt.Errorf("building generate command: %w", err)
	}

	return nil
}
