package main

import (
	"fmt"
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
	Use:   "pack [<manifest>]",
	Short: "Build an .spkg out of a .yaml manifest",
	Long: cli.Dedent(`
		Build an .spkg out of a .yaml manifest. The manifest is optional as it will try to find a file named
		'substreams.yaml' in current working directory if nothing entered. You may enter a directory that contains a
		'substreams.yaml' file in place of '<manifest_file>', or a link to a remote .spkg file, using urls gs://, http(s)://, ipfs://, etc.'.
	`),
	RunE:         runPack,
	Args:         cobra.RangeArgs(0, 1),
	SilenceUsage: true,
}

func init() {
	rootCmd.AddCommand(packCmd)
	packCmd.Flags().StringP("output-file", "o", "{manifestDir}/{spkgDefaultName}", cli.FlagDescription(`
		Specifies output file where the generated "spkg" file will be written. You can use template directives when
		specifying the value of the flag. You can use "{manifestDir}" which resolves to manifest's
		directory. You can use "{spkgDefaultName}" which is the pre-computed default name in the form
		"<name>-<version>" where "<name>" is the manifest's "package.name" value ("_" values in the name are
		replaced by "-") and "<version>" is "package.version" value. You can use "{version}" which resolves
		to "package.version".
	`))
	//packCmd.Flags().StringArrayP("config", "c", []string{}, cli.FlagDescription(`path to a configuration file that contains overrides for the manifest`))
}

func runPack(cmd *cobra.Command, args []string) error {
	manifestPath := ""
	if len(args) == 1 {
		manifestPath = args[0]
	}

	manifestReader, err := manifest.NewReader(manifestPath)
	if err != nil {
		return fmt.Errorf("manifest reader: %w", err)
	}

	if !manifestReader.IsLocalManifest() {
		return fmt.Errorf(`"pack" can only be used to pack local manifest file`)
	}

	pkg, _, err := manifestReader.Read()
	if err != nil {
		return fmt.Errorf("reading manifest %q: %w", manifestPath, err)
	}

	originalOutputFile := maybeGetString(cmd, "output-file")
	resolvedOutputFile := resolveOutputFile(originalOutputFile, map[string]string{
		"manifestDir":     filepath.Dir(manifestPath),
		"spkgDefaultName": fmt.Sprintf("%s-%s.spkg", strings.Replace(pkg.PackageMeta[0].Name, "_", "-", -1), pkg.PackageMeta[0].Version),
		"version":         pkg.PackageMeta[0].Version,
	})

	zlog.Debug("resolved output file", zap.String("original", originalOutputFile), zap.String("resolved", resolvedOutputFile))

	outputDir := filepath.Dir(resolvedOutputFile)
	if err := os.MkdirAll(outputDir, os.ModePerm); err != nil {
		return fmt.Errorf("create output directories: %w", err)
	}

	cnt, err := proto.Marshal(pkg)
	if err != nil {
		return fmt.Errorf("marshalling package: %w", err)
	}

	if err := os.WriteFile(resolvedOutputFile, cnt, 0644); err != nil {
		fmt.Println("")
		return fmt.Errorf("writing file: %w", err)
	}

	fmt.Printf("Successfully wrote %q.\n", resolvedOutputFile)

	return nil
}

func resolveOutputFile(input string, bindings map[string]string) string {
	for k, v := range bindings {
		input = strings.ReplaceAll(input, `{`+k+`}`, v)
	}

	return input
}
