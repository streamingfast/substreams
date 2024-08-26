package codegen

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/streamingfast/cli/sflags"
)

var SQLCmd = &cobra.Command{
	Use:   "sql [<manifest_url>]",
	Short: "Generate sql extension from substreams manifest",
	Args:  cobra.RangeArgs(0, 1),
	RunE:  generateSQLEnv,
}

func generateSQLEnv(cmd *cobra.Command, args []string) error {
	manifestPath := ""
	if len(args) == 1 {
		manifestPath = args[0]
	}

	withDevEnv := sflags.MustGetBool(cmd, "with-dev-env")

	err := buildGenerateCommandFromArgs(manifestPath, Sql, withDevEnv)
	if err != nil {
		return fmt.Errorf("building generate command: %w", err)
	}

	return nil
}
