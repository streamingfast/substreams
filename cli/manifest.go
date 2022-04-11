package cli

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/streamingfast/substreams/manifest"
)

var manifestCmd = &cobra.Command{
	Use:          "manifest",
	SilenceUsage: true,
}
var manifestInfoCmd = &cobra.Command{
	Use:          "info [manifest_file]",
	RunE:         runManifestInfo,
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
}

func init() {
	manifestCmd.AddCommand(manifestInfoCmd)

	rootCmd.AddCommand(manifestCmd)
}

func runManifestInfo(cmd *cobra.Command, args []string) error {

	fmt.Println("Manifest Info")

	manifestPath := args[0]
	manif, err := manifest.New(manifestPath)
	if err != nil {
		return fmt.Errorf("read manifest %q: %w", manifestPath, err)
	}

	manifProto, err := manif.ToProto()
	if err != nil {
		return fmt.Errorf("parse manifest to proto%q: %w", manifestPath, err)
	}

	graph, err := manifest.NewModuleGraph(manifProto.Modules)
	if err != nil {
		return fmt.Errorf("create module graph %w", err)
	}

	fmt.Println("Description:", manifProto.GetDescription())
	fmt.Println("Version:", manifProto.GetSpecVersion())
	fmt.Println("----")
	for _, module := range manifProto.Modules {
		fmt.Println("module:", module.Name)
		fmt.Println("Kind:", module.GetKind())
		fmt.Println("Hash:", manifest.HashModuleAsString(manifProto, graph, module))
	}

	return nil
}
