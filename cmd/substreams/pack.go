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
	packCmd.Flags().StringP("output-dir", "o", ".", cli.FlagDescription(`
		Optional flag to specify output directory to output generated .spkg file. 
		If a local path is supplied it will be relative to the supplied manifest path.
	`))
}

func runPack(cmd *cobra.Command, args []string) error {
	outputDir := maybeGetString(cmd, "output-dir")

	manifestPath := args[0]

	if outputDir != "" {
		if !filepath.IsAbs(outputDir) {
			workingDirectory, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("can't retrieve current directory information")
			}
			newOutputDir := filepath.Join(filepath.Dir(filepath.Join(workingDirectory, manifestPath)), outputDir)
			fmt.Printf("Output directory specified: %s \nThis will be treated as a local path.\nFull folderpath: %s\n\n", outputDir, newOutputDir)
			outputDir = newOutputDir
		} else {
			fmt.Printf("Output directory treated as an absolute path.\nFull folderpath: %s\n\n", outputDir)
		}

		err := os.MkdirAll(outputDir, os.ModePerm)
		if err != nil {
			return fmt.Errorf("error creating output directory: %w", err)
		}
	}

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

	outputFile := filepath.Join(outputDir, defaultFilename)
	if err := ioutil.WriteFile(outputFile, cnt, 0644); err != nil {
		fmt.Println("")
		return fmt.Errorf("writing %w", err)
	}

	fmt.Printf(`To generate bindings for your code:
substream protogen %s

`, defaultFilename)
	fmt.Printf("----------------------------------------\n")
	fmt.Printf("Successfully wrote %q.\n", defaultFilename)

	return nil
}
