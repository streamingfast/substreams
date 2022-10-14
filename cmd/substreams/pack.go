package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/streamingfast/cli"
	"github.com/streamingfast/substreams/manifest"
	"go.uber.org/zap"
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
	packCmd.Flags().StringP("output-dir", "o", "src/pb", cli.FlagDescription(`
		Optional flag to specify output directory to output generated .spkg file. If the received <package> argument is a local Substreams manifest file
		(e.g. a local file ending with .yaml), the output folder will be made relative to it
	`))
}

func runPack(cmd *cobra.Command, args []string) error {
	outputDir := maybeGetString(cmd, "output-dir")

	validOutputDirectorySpecified := true
	fileInfo, err := os.Stat(outputDir)
	if err != nil || !fileInfo.IsDir() {
		fmt.Println("WARNING: Output directory specified is invalid - falling back to default path!\nOutputDir=", outputDir)
		fmt.Println("")
		validOutputDirectorySpecified = false
	}

	manifestPath := args[0]

	manifestReader := manifest.NewReader(manifestPath)

	if validOutputDirectorySpecified && manifestReader.IsLocalManifest() && !filepath.IsAbs(outputDir) {
		newOutputDir := filepath.Join(filepath.Dir(manifestPath), outputDir)
		zlog.Debug("manifest path is a local manifest, making output folder relative to it", zap.String("old", outputDir), zap.String("new", newOutputDir))
		outputDir = newOutputDir
	}

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

	if validOutputDirectorySpecified {
		outputFile := filepath.Join(outputDir, defaultFilename)
		if err := ioutil.WriteFile(outputFile, cnt, 0644); err != nil {
			fmt.Println("")
			return fmt.Errorf("writing %q: %w", defaultFilename, err)
		}
	} else {
		if err := ioutil.WriteFile(defaultFilename, cnt, 0644); err != nil {
			fmt.Println("")
			return fmt.Errorf("writing %q: %w", defaultFilename, err)
		}
	}

	fmt.Printf(`To generate bindings for your code:
substream protogen %s

`, defaultFilename)
	fmt.Printf("----------------------------------------\n")
	fmt.Printf("Successfully wrote %q.\n", defaultFilename)

	return nil
}
