package main

import (
	"fmt"
	"gopkg.in/yaml.v3"
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
	packCmd.Flags().StringArrayP("config", "c", []string{}, cli.FlagDescription(`path to a configuration file that contains overrides for the manifest`))
}

func runPack(cmd *cobra.Command, args []string) error {
	manifestPath := ""
	if len(args) == 1 {
		manifestPath = args[0]
	}

	// Get the value of the -c flag
	overridePaths, _ := cmd.Flags().GetStringArray("config")

	var manifestReaderOptions []manifest.Option

	// If the overridePath is provided, read, decode it and add to manifestReaderOptions
	if len(overridePaths) > 0 {
		var overrides []*manifest.ConfigurationOverride
		for _, overridePath := range overridePaths {
			overrideConfig, err := readOverrideConfig(overridePath)
			if err != nil {
				return fmt.Errorf("reading override config %q: %w", overridePath, err)
			}
			overrides = append(overrides, overrideConfig)
		}
		manifestReaderOptions = append(manifestReaderOptions, manifest.WithOverrides(overrides...))
	}

	// Use the manifestReaderOptions while creating the manifest reader
	manifestReader, err := manifest.NewReader(manifestPath, manifestReaderOptions...)
	if err != nil {
		return fmt.Errorf("manifest reader: %w", err)
	}

	if !manifestReader.IsLocalManifest() {
		return fmt.Errorf(`"pack" can only be use to pack local manifest file`)
	}

	pkg, err := manifestReader.Read()
	if err != nil {
		return fmt.Errorf("reading manifest %q: %w", manifestPath, err)
	}

	if _, err = manifest.NewModuleGraph(pkg.Modules.Modules); err != nil {
		return fmt.Errorf("processing module graph %w", err)
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

	if err := ioutil.WriteFile(resolvedOutputFile, cnt, 0644); err != nil {
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

// This function reads and decodes the override configuration from a given path
func readOverrideConfig(path string) (*manifest.ConfigurationOverride, error) {
	fileBytes, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	overrideConfig := &manifest.ConfigurationOverride{}
	err = yaml.Unmarshal(fileBytes, overrideConfig)
	if err != nil {
		return nil, err
	}

	return overrideConfig, nil
}
