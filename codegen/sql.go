package codegen

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/streamingfast/cli/sflags"
)

var SQLCmd = &cobra.Command{
	Use:   "sql <manifest_url> <module_name>",
	Short: "Generate sql dev environment from substreams manifest",
	Args:  cobra.ExactArgs(3),
	RunE:  generateSQLEnv,
}

func generateSQLEnv(cmd *cobra.Command, args []string) error {
	manifestPath := args[0]
	withDevEnv := sflags.MustGetBool(cmd, "with-dev-env")

	// TODO: create sql.rs template file which will be used to output the database entity changes
	err := buildGenerateCommandFromArgs(manifestPath, Subgraph, withDevEnv)
	if err != nil {
		return fmt.Errorf("building generate command: %w", err)
	}

	return nil
}
