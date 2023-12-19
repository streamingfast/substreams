package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/streamingfast/cli/sflags"
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
	inspectCmd.Flags().Bool("skip-package-validation", false, "Do not perform any validation when reading substreams package")
}

func runInspect(cmd *cobra.Command, args []string) error {
	manifestPath := args[0]

	var readerOptions []manifest.Option
	if sflags.MustGetBool(cmd, "skip-package-validation") {
		readerOptions = append(readerOptions, manifest.SkipPackageValidationReader())
	}

	manifestReader, err := manifest.NewReader(manifestPath, readerOptions...)
	if err != nil {
		return fmt.Errorf("manifest reader: %w", err)
	}

	pkg, _, err := manifestReader.Read()
	if err != nil {
		return fmt.Errorf("reading manifest %q: %w", manifestPath, err)
	}

	filename := filepath.Join(os.TempDir(), "package.spkg")

	cnt, err := proto.Marshal(pkg)
	if err != nil {
		return fmt.Errorf("marshalling package: %w", err)
	}

	if err := os.WriteFile(filename, cnt, 0644); err != nil {
		fmt.Println("")
		return fmt.Errorf("writing %q: %w", filename, err)
	}

	c := exec.Command("protoc", "--decode=sf.substreams.v1.Package", "--descriptor_set_in="+filename)
	c.Stdin = bytes.NewBuffer(cnt)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return c.Run()
}
