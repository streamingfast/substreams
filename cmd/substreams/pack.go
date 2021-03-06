package main

import (
	"fmt"
	"io/ioutil"
	"strings"

	"github.com/spf13/cobra"
	"github.com/streamingfast/substreams/manifest"
	"google.golang.org/protobuf/proto"
)

var packCmd = &cobra.Command{
	Use:          "pack <package>",
	Short:        "Build an .spkg out of a .yaml manifest",
	RunE:         runPack,
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
}

func init() {
	rootCmd.AddCommand(packCmd)
}

func runPack(cmd *cobra.Command, args []string) error {
	manifestPath := args[0]

	manifestReader := manifest.NewReader(manifestPath)
	pkg, err := manifestReader.Read()
	if err != nil {
		return fmt.Errorf("reading manifest %q: %w", manifestPath, err)
	}

	if _, err = manifest.NewModuleGraph(pkg.Modules.Modules); err != nil {
		return fmt.Errorf("processing module graph %w", err)
	}

	defaultFilename := fmt.Sprintf("%s-%s.spkg", strings.Replace(pkg.PackageMeta[0].Name, "_", "-", -1), pkg.PackageMeta[0].Version)

	cnt, err := proto.Marshal(pkg)
	if err != nil {
		return fmt.Errorf("marshalling package: %w", err)
	}

	if err := ioutil.WriteFile(defaultFilename, cnt, 0644); err != nil {
		fmt.Println("")
		return fmt.Errorf("writing %q: %w", defaultFilename, err)
	}

	fmt.Printf(`To generate bindings for your code:
substream protogen %s

`, defaultFilename)
	fmt.Printf("----------------------------------------\n")
	fmt.Printf("Successfully wrote %q.\n", defaultFilename)

	return nil
}
