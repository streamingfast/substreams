package tools

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/streamingfast/cli/sflags"
	"github.com/streamingfast/substreams/manifest"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

var extractWASMCmd = &cobra.Command{
	Use:   "extract-wasm <spkg-url> [dest-wasm-file]",
	Short: "extract a wasm binary from a substreams package, useful when publishing a new release of a substreams package without changing the hashes",
	Args:  cobra.RangeArgs(1, 2),
	RunE:  extractWASME,
}

func init() {
	extractWASMCmd.Flags().String("module", "", "module name to extract")
	Cmd.AddCommand(extractWASMCmd)
}

func extractWASME(cmd *cobra.Command, args []string) error {
	src := args[0]
	dest := "extracted.wasm"
	if len(args) == 2 {
		dest = args[1]
	}

	module := sflags.MustGetString(cmd, "module")

	manifestReader, err := manifest.NewReader(src, manifest.SkipPackageValidationReader())
	if err != nil {
		return fmt.Errorf("manifest reader: %w", err)
	}

	pkgBundle, err := manifestReader.Read()
	if err != nil {
		return fmt.Errorf("read manifest %q: %w", src, err)
	}

	if pkgBundle == nil {
		return fmt.Errorf("no package found")
	}

	var bin *pbsubstreams.Binary
	switch {
	case module != "":
		for _, mod := range pkgBundle.Package.Modules.Modules {
			if mod.Name == module {
				bin = pkgBundle.Package.Modules.Binaries[mod.BinaryIndex]
				break
			}
		}
		if bin == nil {
			return fmt.Errorf("module %q not found", module)
		}
	case len(pkgBundle.Package.Modules.Binaries) == 1:
		bin = pkgBundle.Package.Modules.Binaries[0]
	default:
		return fmt.Errorf("multiple binaries found, please specify a module name")
	}

	if err := os.WriteFile(dest, bin.Content, 0644); err != nil {
		return fmt.Errorf("write file %q: %w", dest, err)
	}
	fmt.Printf("WASM extracted to file %s (%s) (%d bytes)\n", dest, bin.Type, len(bin.Content))

	fmt.Println("\nTo use in all your modules, replace the file that is set as 'default' under 'binaries' in your substreams.yaml file.")
	fmt.Println("To use only on some of your modules, add the following:")
	fmt.Printf(`
binaries:
  extracted:
    type: %s
    file: %s
`,
		bin.Type, dest)
	fmt.Printf("\nand set the `binary` field in each module to the name of the binary, e.g. `extracted`\n\n")

	return nil
}
