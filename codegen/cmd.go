package codegen

import (
	"github.com/spf13/cobra"
	"github.com/streamingfast/substreams/codegen/subgraph"
)

var Cmd = &cobra.Command{Use: "codegen", Short: "Code generator for substreams"}

func init() {
	Cmd.AddCommand(subgraph.SubgraphCmd)
}
