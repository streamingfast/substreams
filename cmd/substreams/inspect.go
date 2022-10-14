package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/streamingfast/substreams/manifest"
	"google.golang.org/protobuf/proto"
)

var inspectCmd = &cobra.Command{
	Use:          "inspect <package>",
	Short:        "Display low-level package structure",
	RunE:         runInspect,
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
}

func init() {
	rootCmd.AddCommand(inspectCmd)
}

func runInspect(cmd *cobra.Command, args []string) error {
	manifestPath := args[0]

	manifestReader := manifest.NewReader(manifestPath)
	pkg, err := manifestReader.Read()
	if err != nil {
		return fmt.Errorf("reading manifest %q: %w", manifestPath, err)
	}

	if _, err = manifest.NewModuleGraph(pkg.Modules.Modules); err != nil {
		return fmt.Errorf("processing module graph %w", err)
	}

	filename := filepath.Join(os.TempDir(), "package.spkg")

	cnt, err := proto.Marshal(pkg)
	if err != nil {
		return fmt.Errorf("marshalling package: %w", err)
	}

	if err := ioutil.WriteFile(filename, cnt, 0644); err != nil {
		fmt.Println("")
		return fmt.Errorf("writing %q: %w", filename, err)
	}

	c := exec.Command("protoc", "--decode=sf.substreams.v1.Package", "--descriptor_set_in="+filename)
	c.Stdin = bytes.NewBuffer(cnt)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return c.Run()
}
