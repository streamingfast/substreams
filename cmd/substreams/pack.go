package main

import (
	"fmt"
	"io/ioutil"

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

	defaultFilename := fmt.Sprintf("%s-%s.spkg", pkg.PackageMeta[0].Name, pkg.PackageMeta[0].Version)
	cnt, err := proto.Marshal(pkg)
	if err != nil {
		return fmt.Errorf("marshalling package: %w", err)
	}

	fmt.Printf("Writing %q... ", defaultFilename)
	if err := ioutil.WriteFile(defaultFilename, cnt, 0644); err != nil {
		fmt.Println("")
		return fmt.Errorf("writing %q: %w", defaultFilename, err)
	}

	fmt.Println(`To generate bindings for your Rust code:
1. create a file 'buf.gen.yaml' with this content:

version: v1
plugins:
  - name: prost
    out: gen/src

2. run 'buf generate /path/to/bundle.spkg#format=bin'`)

	fmt.Println("done")

	return nil
}
