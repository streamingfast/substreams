package cli

import (
	"fmt"
	"io/ioutil"

	"github.com/spf13/cobra"
	"github.com/streamingfast/substreams/manifest"
	"google.golang.org/protobuf/proto"
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

var manifestPackageCmd = &cobra.Command{
	Use:          "package [manifest_yaml] {manifest_pkg_pb}",
	RunE:         runManifestPackage,
	Args:         cobra.RangeArgs(1, 2),
	SilenceUsage: true,
}

func init() {
	manifestCmd.AddCommand(manifestInfoCmd)
	manifestCmd.AddCommand(manifestPackageCmd)

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

func runManifestPackage(cmd *cobra.Command, args []string) error {
	manifestPath := args[0]
	manif, err := manifest.New(manifestPath)
	if err != nil {
		return fmt.Errorf("reading manifest %q: %w", manifestPath, err)
	}

	manifProto, err := manif.ToProto()
	if err != nil {
		return fmt.Errorf("parsing manifest %q into protobuf: %w", manifestPath, err)
	}

	// TODO: do all the validation on the proto too

	if _, err = manifest.NewModuleGraph(manifProto.Modules); err != nil {
		return fmt.Errorf("processing module graph %w", err)
	}

	var outputFile string
	if len(args) == 2 {
		outputFile = args[1]
	} else {
		outputFile = manifestPath + ".pb"
	}

	cnt, err := proto.Marshal(manifProto)
	if err != nil {
		return fmt.Errorf("marshalling manifest proto: %w", err)
	}

	fmt.Printf("Writing %q... ", outputFile)
	if err := ioutil.WriteFile(outputFile, cnt, 0644); err != nil {
		fmt.Println("")
		return fmt.Errorf("writing %q: %w", outputFile, err)
	}

	fmt.Println("done")

	return nil
}
