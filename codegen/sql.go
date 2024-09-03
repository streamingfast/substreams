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

	input := fmt.Sprintf(
		"Your Substreams SQL Extension is now generated!\n\n" +
			"**Now follow the next steps:**\n\n" +
			"Open the sql directory:\n\n" +
			"`cd sql`\n\n" +
			"Build the substreams package:\n\n" +
			"`substreams build`\n\n" +
			"Sink data into your database:\n\n" +
			"`substreams-sink-sql`\n\n",
	)
	printMarkdown(input)

	return nil
}
