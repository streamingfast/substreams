package codegen

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/streamingfast/cli/sflags"
)

var SQLCmd = &cobra.Command{
	Use:   "subgraph <manifest_url> <module_name>",
	Short: "Generate subgraph dev environment from substreams manifest",
	Args:  cobra.ExactArgs(3),
	RunE:  generateSQLEnv,
}

func generateSQLEnv(cmd *cobra.Command, args []string) error {
	manifestPath := args[0]
	withDevEnv := sflags.MustGetBool(cmd, "with-dev-env")

	err := buildGenerateCommandFromArgs(manifestPath, outputTypeSubgraph, withDevEnv)
	if err != nil {
		return fmt.Errorf("building generate command: %w", err)
	}

	return nil
}
