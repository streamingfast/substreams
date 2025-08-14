package main

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/streamingfast/cli"
	"github.com/streamingfast/substreams/manifest"
	"go.uber.org/zap"
)

var registryVerify = &cobra.Command{
	Use:   "verify [github_release_url | https_spkg_path | local_spkg_path | local_substreams_path]",
	Short: "Verify a package is ready for publishing to the Substreams.dev registry",
	Long: cli.Dedent(`
		Verify a package is ready for publishing to the Substreams.dev registry. This command performs
		all the validation checks that would be done during publishing, but does not actually publish
		the package.

		You can specify a GitHub release URL, HTTPS spkg path, local spkg path, or local substreams path.
		If no argument is provided, it will look for a substreams.yaml file in the current directory.
		You can use "-" to read the manifest from standard input.
	`),
	Args: cobra.MaximumNArgs(1),
	RunE: runRegistryVerify,
}

func init() {
	registryCmd.AddCommand(registryVerify)
}

func runRegistryVerify(cmd *cobra.Command, args []string) (err error) {
	var manifestPath string
	switch len(args) {
	case 0:
		manifestPath, err = resolveManifestFile("")
		if err != nil {
			return fmt.Errorf("resolving manifest: %w", err)
		}
	case 1:
		manifestPath = args[0]
	}

	spkgRegistry := getSubstreamsDownloadEndpoint()

	readerOptions := []manifest.Option{
		manifest.WithRegistryURL(spkgRegistry),
	}

	manifestReader, err := manifest.NewReader(manifestPath, readerOptions...)
	if err != nil {
		return fmt.Errorf("manifest reader: %w", err)
	}

	pkgBundle, err := manifestReader.Read()
	if err != nil {
		return fmt.Errorf("read manifest %q: %w", manifestPath, err)
	}

	spkg := pkgBundle.Package

	// Validate proto output types
	if err := manifest.ValidateProtoOutputTypes(spkg); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	// Check for warnings about incomplete package
	warned := warnIncompletePackage(spkg, warningsConfig{})
	
	// Print package details
	printPackageDetails(spkg)

	if warned {
		fmt.Println("⚠️ Package has warnings but is valid for publishing")
	} else {
		fmt.Println("✅ Package is valid and ready for publishing")
	}
	
	fmt.Println()
	fmt.Println("To publish this package, run:")
	fmt.Println()
	fmt.Println(cli.PurpleStyle.Render("    substreams registry publish"))
	fmt.Println()

	zlog.Debug("package verification completed successfully")
	return nil
}

