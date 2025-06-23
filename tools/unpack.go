package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/streamingfast/substreams/manifest"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"google.golang.org/protobuf/proto"
	"gopkg.in/yaml.v3"
)

var unpackCmd = &cobra.Command{
	Use:   "unpack <spkg-url>",
	Short: "extract substreams.yaml and binary files from a substreams package into a folder",
	Long: `Unpack extracts a substreams package into a folder named '{packagename}-unpacked' containing:
- substreams.yaml: The manifest file
- Binary files: All WASM and other binary files referenced in the manifest
Useful for tweaking an existing package`,
	Args: cobra.ExactArgs(1),
	RunE: unpackE,
}

func init() {
	Cmd.AddCommand(unpackCmd)
}

func unpackE(cmd *cobra.Command, args []string) error {
	src := args[0]

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

	// Get package name for the output directory
	packageName := "unknown"
	if pkgBundle.Package.PackageMeta != nil && len(pkgBundle.Package.PackageMeta) > 0 {
		if name := pkgBundle.Package.PackageMeta[0].Name; name != "" {
			packageName = name
		}
	}

	// Create output directory
	outputDir := fmt.Sprintf("%s-unpacked", packageName)
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("create output directory %q: %w", outputDir, err)
	}

	// Write a copy of the spkg as protodesc reference
	spkgFilename, err := writeSpkgFromPackage(outputDir, packageName, pkgBundle)
	if err != nil {
		return fmt.Errorf("create spkg file: %w", err)
	}

	// Extract substreams.yaml
	if err := extractManifest(pkgBundle, outputDir, spkgFilename); err != nil {
		return fmt.Errorf("extract manifest: %w", err)
	}

	// Extract binary files
	if err := extractBinaries(pkgBundle, outputDir); err != nil {
		return fmt.Errorf("extract binaries: %w", err)
	}

	fmt.Printf("Package unpacked to directory: %s\n", outputDir)
	fmt.Printf("Contents:\n")
	fmt.Printf("  - substreams.yaml: manifest file\n")
	fmt.Printf("  - %s: original package file (protobuf descriptor set)\n", spkgFilename)

	binariesCount := len(pkgBundle.Package.Modules.Binaries)
	if binariesCount > 0 {
		fmt.Printf("  - %d binary file(s) extracted\n", binariesCount)
	}

	fmt.Printf("\nNote: This package contains compiled binaries and protobuf definitions.\n")
	fmt.Printf("The extracted files can be used to:\n")
	fmt.Printf("  - Inspect the manifest structure\n")
	fmt.Printf("  - Extract binary files for other uses\n")
	fmt.Printf("  - Run substreams commands directly: substreams run %s <module_name>\n", outputDir)
	fmt.Printf("\nNote: The .spkg file is recreated from the loaded package data.\n")
	fmt.Printf("To rebuild from source, you would need the original Rust source code and Cargo.toml.\n")

	return nil
}

func writeSpkgFromPackage(outputDir, packageName string, pkgBundle *manifest.PackageBundle) (string, error) {
	// Determine the spkg filename
	version := "unknown"
	if len(pkgBundle.Package.PackageMeta) > 0 && pkgBundle.Package.PackageMeta[0].Version != "" {
		version = pkgBundle.Package.PackageMeta[0].Version
	}
	spkgFilename := fmt.Sprintf("%s@%s.spkg", packageName, version)

	// Serialize the package object to protobuf
	data, err := proto.Marshal(pkgBundle.Package)
	if err != nil {
		return spkgFilename, fmt.Errorf("marshal package to protobuf: %w", err)
	}

	// Write the spkg file
	destPath := filepath.Join(outputDir, spkgFilename)
	if err := os.WriteFile(destPath, data, 0644); err != nil {
		return spkgFilename, fmt.Errorf("write spkg file %q: %w", destPath, err)
	}

	return spkgFilename, nil
}

func extractManifest(pkgBundle *manifest.PackageBundle, outputDir, spkgFilename string) error {
	var manifestData []byte
	var err error

	// If we have the original manifest, use it and update binary paths
	if pkgBundle.Manifest != nil {

		// Create a copy of the manifest to modify
		manifestCopy := *pkgBundle.Manifest

		// Add protobuf descriptor set reference if not already present
		if manifestCopy.Protobuf.DescriptorSets == nil {
			manifestCopy.Protobuf = manifest.Protobuf{
				DescriptorSets: []*manifest.BufImport{
					{
						LocalPath: spkgFilename,
						Module:    "local",  // Placeholder for local files
						Version:   "v1.0.0", // Placeholder for local files
					},
				},
			}
		}

		// Update binary file paths to point to extracted files
		if manifestCopy.Binaries != nil {
			for name, binary := range manifestCopy.Binaries {
				ext := getFileExtensionFromBinaryType(binary.Type)
				if len(manifestCopy.Binaries) == 1 {
					binary.File = fmt.Sprintf("binary%s", ext)
				} else {
					// Find the index of this binary in the package
					for i, pkgBinary := range pkgBundle.Package.Modules.Binaries {
						if pkgBinary.Type == binary.Type {
							binary.File = fmt.Sprintf("binary_%d%s", i, ext)
							break
						}
					}
				}
				manifestCopy.Binaries[name] = binary
			}
		}

		manifestData, err = yaml.Marshal(&manifestCopy)
		if err != nil {
			return fmt.Errorf("marshal manifest to YAML: %w", err)
		}
	} else {

		// Fallback: create a minimal manifest from package data
		manifestData, err = createManifestFromPackage(pkgBundle.Package, spkgFilename)
		if err != nil {
			return fmt.Errorf("create manifest from package: %w", err)
		}
	}

	manifestPath := filepath.Join(outputDir, "substreams.yaml")
	if err := os.WriteFile(manifestPath, manifestData, 0644); err != nil {
		return fmt.Errorf("write manifest file %q: %w", manifestPath, err)
	}

	return nil
}

func extractBinaries(pkgBundle *manifest.PackageBundle, outputDir string) error {
	binaries := pkgBundle.Package.Modules.Binaries
	if len(binaries) == 0 {
		return nil
	}

	// Create a map of binary types to count for unique naming
	typeCount := make(map[string]int)

	for i, binary := range binaries {
		// Determine file extension based on binary type
		ext := getFileExtensionFromBinaryType(binary.Type)

		// Create unique filename
		var filename string
		typeCount[binary.Type]++
		if typeCount[binary.Type] == 1 && len(binaries) == 1 {
			filename = fmt.Sprintf("binary%s", ext)
		} else {
			filename = fmt.Sprintf("binary_%d%s", i, ext)
		}

		filePath := filepath.Join(outputDir, filename)
		if err := os.WriteFile(filePath, binary.Content, 0644); err != nil {
			return fmt.Errorf("write binary file %q: %w", filePath, err)
		}

		fmt.Printf("  - %s: %s (%d bytes)\n", filename, binary.Type, len(binary.Content))
	}

	return nil
}

func getFileExtensionFromBinaryType(binaryType string) string {
	// Split binary type to get the base type (before any '+' extensions)
	baseType := strings.Split(binaryType, "+")[0]

	switch {
	case strings.Contains(baseType, "wasm"):
		return ".wasm"
	case strings.Contains(baseType, "native"):
		return ".so"
	default:
		return ".bin"
	}
}

func createManifestFromPackage(pkg *pbsubstreams.Package, spkgFilename string) ([]byte, error) {
	// This is a fallback function to create a basic manifest from package data
	// when the original manifest is not available

	manif := &manifest.Manifest{
		SpecVersion: "v0.1.0",
	}

	// Add protobuf descriptor set reference
	manif.Protobuf = manifest.Protobuf{
		DescriptorSets: []*manifest.BufImport{
			{
				LocalPath: spkgFilename,
				Module:    "local",  // Placeholder for local files
				Version:   "v1.0.0", // Placeholder for local files
			},
		},
	}

	// Set package metadata
	if len(pkg.PackageMeta) > 0 {
		meta := pkg.PackageMeta[0]
		manif.Package = manifest.PackageMeta{
			Name:        meta.Name,
			Version:     meta.Version,
			URL:         meta.Url,
			Doc:         meta.Doc,
			Description: meta.Description,
		}
	}

	// Add binaries
	manif.Binaries = make(map[string]manifest.Binary)
	for i, binary := range pkg.Modules.Binaries {
		ext := getFileExtensionFromBinaryType(binary.Type)
		filename := fmt.Sprintf("binary_%d%s", i, ext)

		binaryName := fmt.Sprintf("binary_%d", i)
		if len(pkg.Modules.Binaries) == 1 {
			binaryName = "default"
			filename = fmt.Sprintf("binary%s", ext)
		}

		manif.Binaries[binaryName] = manifest.Binary{
			Type: binary.Type,
			File: filename,
		}
	}

	// Initialize params map for parameter references
	manif.Params = make(map[string]string)

	// Add modules (basic conversion)
	for _, module := range pkg.Modules.Modules {
		mod := &manifest.Module{
			Name: module.Name,
		}

		// Set module kind and type-specific fields
		switch kind := module.Kind.(type) {
		case *pbsubstreams.Module_KindMap_:
			mod.Kind = "map"
			if module.Output != nil {
				mod.Output = manifest.StreamOutput{
					Type: module.Output.Type,
				}
			}
		case *pbsubstreams.Module_KindStore_:
			mod.Kind = "store"
			mod.UpdatePolicy = strings.ToLower(strings.TrimPrefix(kind.KindStore.UpdatePolicy.String(), "UPDATE_POLICY_"))
			mod.ValueType = kind.KindStore.ValueType
		case *pbsubstreams.Module_KindBlockIndex_:
			mod.Kind = "blockIndex"
			if module.Output != nil {
				mod.Output = manifest.StreamOutput{
					Type: module.Output.Type,
				}
			}
		}

		// Set binary reference
		if module.BinaryIndex < uint32(len(pkg.Modules.Binaries)) {
			if len(pkg.Modules.Binaries) == 1 {
				mod.Binary = "default"
			} else {
				mod.Binary = fmt.Sprintf("binary_%d", module.BinaryIndex)
			}
		}

		// Set initial block
		if module.InitialBlock > 0 {
			mod.InitialBlock = &module.InitialBlock
		}

		// Convert inputs
		for _, input := range module.Inputs {
			manifestInput := &manifest.Input{}

			switch inp := input.Input.(type) {
			case *pbsubstreams.Module_Input_Source_:
				manifestInput.Source = inp.Source.Type
			case *pbsubstreams.Module_Input_Map_:
				manifestInput.Map = inp.Map.ModuleName
			case *pbsubstreams.Module_Input_Store_:
				manifestInput.Store = inp.Store.ModuleName
				manifestInput.Mode = strings.ToLower(inp.Store.Mode.String())
			case *pbsubstreams.Module_Input_Params_:
				// Use 'string' as the parameter type, but store the value under the module name
				manifestInput.Params = "string"
				// Store the actual parameter value using the module name as the key
				manif.Params[module.Name] = inp.Params.Value
			}

			mod.Inputs = append(mod.Inputs, manifestInput)
		}

		manif.Modules = append(manif.Modules, mod)
	}

	return yaml.Marshal(manif)
}
