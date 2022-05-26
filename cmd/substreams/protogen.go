package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
	"github.com/streamingfast/substreams/manifest"
	"google.golang.org/protobuf/proto"
)

var protogenCmd = &cobra.Command{
	Use:          "protogen <manifest_yaml> [manifest_spkg]",
	RunE:         runProtogen,
	Args:         cobra.RangeArgs(1, 2),
	SilenceUsage: true,
}

func init() {
	rootCmd.AddCommand(protogenCmd)
	protogenCmd.Flags().StringP("output-path", "o", "src/pb", "Directory to output generated .rs files")
}

func runProtogen(cmd *cobra.Command, args []string) error {
	outputPath := mustGetString(cmd, "output-path")
	manifestPath := args[0]
	pkg, err := manifest.New(manifestPath)
	if err != nil {
		return fmt.Errorf("reading manifest %q: %w", manifestPath, err)
	}

	// write the manifest to temp location
	// write buf.gen.yaml with custom stuff
	// run `buf generate`
	// remove if we wrote buf.gen.yaml (--keep-buf-gen-yaml)

	if _, err = manifest.NewModuleGraph(pkg.Modules.Modules); err != nil {
		return fmt.Errorf("processing module graph %w", err)
	}

	defaultFilename := "/tmp/tmp.spkg"
	cnt, err := proto.Marshal(pkg)
	if err != nil {
		return fmt.Errorf("marshalling package: %w", err)
	}

	if err := ioutil.WriteFile(defaultFilename, cnt, 0644); err != nil {
		fmt.Println("")
		return fmt.Errorf("writing %q: %w", defaultFilename, err)
	}

	_, err = os.Stat("buf.gen.yaml")
	bufFileFound := err != os.ErrNotExist

	if !bufFileFound {
		fmt.Println("Writing a temporary 'buf.gen.yaml'")
		if err := ioutil.WriteFile("buf.gen.yaml", []byte(`
version: v1
plugins:
  - name: prost
    out: `+outputPath+`
    opt:
      - bytes=.
`), 0644); err != nil {
			return fmt.Errorf("error writing buf.gen.yaml: %w", err)
		}
	}

	fmt.Println("Running: buf generate /tmp/tmp.spkg#format=bin")
	c := exec.Command("buf", "generate", "/tmp/tmp.spkg#format=bin")
	c.Stdin = os.Stdin
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	if err := c.Run(); err != nil {
		return fmt.Errorf("error executing 'buf':: %w", err)
	}

	if !bufFileFound {
		fmt.Println("Removing temporary 'buf.gen.yaml'")
		if err := os.Remove("buf.gen.yaml"); err != nil {
			fmt.Errorf("error delefing buf.gen.yaml: %w", err)
		}
	}

	fmt.Println("Done")

	return nil
}
