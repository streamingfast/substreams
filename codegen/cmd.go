package codegen

import (
	"github.com/spf13/cobra"
)

var Cmd = &cobra.Command{Use: "codegen", Short: "Code generator for substreams"}

func init() {
	SQLCmd.Flags().Bool("with-dev-env", false, "generate sql extension dev environment")

	Cmd.AddCommand(SQLCmd)
}
