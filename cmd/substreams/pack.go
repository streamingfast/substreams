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
	Use:          "pack <manifest_yaml> [manifest_spkg]",
	RunE:         runPack,
	Args:         cobra.RangeArgs(1, 2),
	SilenceUsage: true,
}

func init() {
	rootCmd.AddCommand(packCmd)
}

func runPack(cmd *cobra.Command, args []string) error {
	manifestPath := args[0]
	pkg, err := manifest.New(manifestPath)
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

	fmt.Printf(`To generate bindings for your Rust code:
1. create a file 'buf.gen.yaml' with this content:

version: v1
plugins:
  - name: prost
    out: gen/src
    opt:
      - bytes=.
      - compile_well_known_types
2. run 'buf generate %s#format=bin'

3. See https://crates.io/crates/protoc-gen-prost for more details
`, defaultFilename)
	fmt.Println("")
	fmt.Printf("----------------------------------------\n")
	fmt.Printf("Successfully wrote %q.\n", defaultFilename)

	return nil
}
