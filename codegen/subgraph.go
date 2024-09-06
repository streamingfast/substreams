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

	input := fmt.Sprintf(
		"Your Subgraph Powered Subtreams is now generated!\n\n" +
			"**Now follow the next steps:**\n\n" +
			"Open the subgraph directory:\n\n" +
			"`cd subgraph`\n\n" +
			"Install all the dependencies:\n\n" +
			"`npm install`\n\n" +
			"Generate AssemblyScript types:\n\n" +
			"`npm run codegen`\n\n" +
			"Generate protobuf:\n\n" +
			"`npm run protogen`\n\n" +
			"Build your project:\n\n" +
			"`npm run build`\n\n" +
			"Create the subgraph:\n\n" +
			"`npm run create-local`\n\n" +
			"Deploy the subgraph:\n\n" +
			"`npm run deploy-local`\n\n",
	)
	fmt.Println(ToMarkdown(input))
	return nil
}
