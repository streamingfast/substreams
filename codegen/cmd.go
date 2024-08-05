package codegen

import (
	"github.com/spf13/cobra"
)

var Cmd = &cobra.Command{Use: "codegen", Short: "Code generator for substreams"}

func init() {
	SubgraphCmd.Flags().Bool("with-dev-env", false, "generate graph node dev environment")

	Cmd.AddCommand(SubgraphCmd)
	Cmd.AddCommand(SQLCmd)
}
