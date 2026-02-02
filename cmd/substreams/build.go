package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/streamingfast/cli"
	"github.com/streamingfast/cli/sflags"
	"github.com/streamingfast/substreams/codegen"
	"github.com/streamingfast/substreams/manifest"
	"gopkg.in/yaml.v3"
)

var buildCmd = &cobra.Command{
	Use:   "build",
	Short: "Build the project according to substreams.yaml",
	Long: cli.Dedent(`
		Build the project according to the specifications in substreams.yaml.
		This command will check for dependencies, run the appropriate build commands,
		and handle code generation steps.
	`),
	RunE:         runBuildE,
	SilenceUsage: true,
}

func init() {
	buildCmd.Flags().Bool("no-pack", false, "Do not pack the build output (default false)")
	buildCmd.Flags().String("manifest", "", "Path to the manifest file (use \"-\" to read from stdin)")
	buildCmd.Flags().String("binary", "default", "binary label to build from manifest")
	rootCmd.AddCommand(buildCmd)
}

func runBuildE(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()

	// Parse substreams.yaml
	manifestPath := sflags.MustGetString(cmd, "manifest")
	if manifestPath == "" {
		var err error
		manifestPath, err = findManifest()
		if err != nil {
			return fmt.Errorf("❌ Error finding manifest: %w", err)
		}
	}

	if manifestPath == "-" {
		fmt.Printf("📄 Reading manifest from stdin\n")
	} else if manifestPath != "" {
		fmt.Printf("🏗️  Building Substreams package from %s\n", manifestPath)
	}

	manifestReader, err := manifest.NewReader(manifestPath, manifest.SkipSourceCodeReader())
	if err != nil {
		return fmt.Errorf("❌ Manifest reader: %w", err)
	}

	pkgBundle, err := manifestReader.Read()
	if err != nil {
		return fmt.Errorf("❌ Reading manifest: %w", err)
	}

	// For stdin, we need to create a temporary file for subcommands since stdin can only be read once
	actualPath := manifestPath
	var cleanupFunc func()
	if manifestPath == "-" {
		tempFile, err := createTempManifestFile(pkgBundle.Manifest)
		if err != nil {
			return fmt.Errorf("❌ Creating temporary manifest file: %w", err)
		}
		actualPath = tempFile
		cleanupFunc = func() { os.Remove(tempFile) }
		defer cleanupFunc()
	}

	info := &manifestInfo{
		Path:     actualPath,
		Manifest: *pkgBundle.Manifest,
	}

	binaryLabel := sflags.MustGetString(cmd, "binary")

	protoBuilder, err := newProtoBuilder(info, binaryLabel)
	if err != nil {
		return fmt.Errorf("❌ Error creating proto builder: %w", err)
	}
	err = protoBuilder.Build(ctx)
	if err != nil {
		return fmt.Errorf("❌ Error running protogen: %w", err)
	}

	binaryBuilder, err := newBinaryBuilder(info, binaryLabel)
	if err != nil {
		return fmt.Errorf("❌ Error creating binary builder: %w", err)
	}

	err = binaryBuilder.Build(ctx)
	if err != nil {
		return fmt.Errorf("❌ Error building binary: %w", err)
	}

	fmt.Println()

	noPack := sflags.MustGetBool(cmd, "no-pack")
	if noPack {
		fmt.Printf("✅ Build complete! Binaries ready for use.\n")
		return nil
	}

	spkgBuilder, err := newSPKGPacker(info)
	if err != nil {
		return fmt.Errorf("❌ Error creating spkg builder: %w", err)
	}

	err = spkgBuilder.Build(ctx)
	if err != nil {
		return fmt.Errorf("❌ Error building spkg: %w", err)
	}

	fmt.Println()
	fmt.Printf("✅ Build complete!\n")
	return nil
}

func createTempManifestFile(manif *manifest.Manifest) (string, error) {
	// Create a temporary file in the current directory
	tmpFile, err := os.CreateTemp(".", "substreams-*.yaml")
	if err != nil {
		return "", err
	}

	defer func() {
		tmpFile.Close()
		if err != nil {
			os.Remove(tmpFile.Name())
		}
	}()

	// Convert manifest back to YAML
	yamlData, err := yaml.Marshal(manif)
	if err != nil {
		return "", fmt.Errorf("marshaling manifest to YAML: %w", err)
	}

	// Write YAML to temporary file
	if _, err := tmpFile.Write(yamlData); err != nil {
		return "", fmt.Errorf("writing manifest to temporary file: %w", err)
	}

	return tmpFile.Name(), nil
}

type manifestInfo struct {
	Path     string
	Manifest manifest.Manifest
}

type ProtoBuilder struct {
	manifInfo   *manifestInfo
	binaryLabel string
}

func newProtoBuilder(manifInfo *manifestInfo, binaryLabel string) (*ProtoBuilder, error) {
	return &ProtoBuilder{
		manifInfo:   manifInfo,
		binaryLabel: binaryLabel,
	}, nil
}

func (p *ProtoBuilder) Build(ctx context.Context) error {
	if len(p.manifInfo.Manifest.Binaries) == 0 || !strings.HasPrefix(p.manifInfo.Manifest.Binaries[p.binaryLabel].Type, "wasm/rust-v1") {
		fmt.Println("⚠️  No Rust binaries found, skipping protobuf generation")
		return nil
	}

	// Read the manifest to get the package
	readerOptions := []manifest.Option{
		manifest.SkipSourceCodeReader(),
		manifest.SkipModuleOutputTypeValidationReader(),
	}

	manifestReader, err := manifest.NewReader(p.manifInfo.Path, readerOptions...)
	if err != nil {
		return fmt.Errorf("manifest reader: %w", err)
	}

	// Determine output path relative to manifest
	outputPath := "src/pb"
	if manifestReader.IsLocalManifest() && !filepath.IsAbs(outputPath) {
		outputPath = filepath.Join(filepath.Dir(p.manifInfo.Path), outputPath)
	}

	pkgBundle, err := manifestReader.Read()
	if err != nil {
		return fmt.Errorf("reading manifest %q: %w", p.manifInfo.Path, err)
	}

	if len(pkgBundle.Manifest.Protobuf.ExcludePaths) == 0 {
		fmt.Printf("⚠️  No protobuf exclude paths configured\n")
		fmt.Printf("   Consider adding 'google' and 'sf/substreams' to exclude paths if build fails\n")
	}

	generator := codegen.NewProtoGenerator(outputPath, pkgBundle.Manifest.Protobuf.ExcludePaths, true)

	// Check for non-deterministic descriptor sets and warn the user
	nonDeterministicEntries := pkgBundle.Manifest.GetNonDeterministicDescriptorSets()
	if len(nonDeterministicEntries) > 0 {
		generator.SetHasNonDeterministicDescriptors(true)
		fmt.Printf("⚠️  Warning: Some descriptor sets do not have a pinned version and will always trigger regeneration:\n")
		for _, entry := range nonDeterministicEntries {
			if entry.Version == "" {
				fmt.Printf("   • %s (no version specified, resolves to latest)\n", entry.Module)
			} else {
				fmt.Printf("   • %s@%s (non-deterministic version)\n", entry.Module, entry.Version)
			}
		}
		fmt.Printf("   To enable caching and faster builds, specify a semver (e.g., @v1.0.0) or commit ref (e.g., @f3ab5976b9ba4f9bac28b26271fca7d7)\n")
	}

	err = generator.GenerateProto(pkgBundle.Package)
	if err != nil {
		return fmt.Errorf("error generating proto: %w", err)
	}

	return nil
}

type BinaryBuilder struct {
	manifInfo   *manifestInfo
	binaryLabel string
}

func newBinaryBuilder(manifInfo *manifestInfo, binaryLabel string) (*BinaryBuilder, error) {
	return &BinaryBuilder{
		manifInfo:   manifInfo,
		binaryLabel: binaryLabel,
	}, nil
}

func (b *BinaryBuilder) isBuildRequired() bool {
	buildRequired := true

	//check modules.  if all have a "use" field, then no build is required
	allUse := true
	for _, mod := range b.manifInfo.Manifest.Modules {
		if mod.Use == "" {
			allUse = false
		}
	}
	if allUse {
		buildRequired = false
	} else {
		buildRequired = true
	}

	return buildRequired
}

func (b *BinaryBuilder) getCmdArgs(ctx context.Context, binaryType string) ([][]string, error) {
	binaryTypeID, _ := manifest.SplitBinaryType(binaryType)

	switch binaryTypeID {
	case "javascript/v8":
		fmt.Printf("🟨 JavaScript/V8 binary detected\n")
		return [][]string{}, nil
	case "wasip1/tinygo-v1":
		fmt.Printf("🟦 TinyGo binary detected\n")
		depValidator := &TinyGoDependencyValidator{}
		err := depValidator.ValidateDependency(ctx)
		if err != nil {
			return nil, fmt.Errorf("validating tinygo dependency: %w", err)
		}

		return [][]string{{"tinygo", "build", "-o", "main.wasm", "-target", "wasi", "-gc", "leaking", "-scheduler", "none", "."}}, nil
	case "wasm/rust-v1":
		fmt.Printf("🦀 Rust binary detected\n")
		depValidator := &CargoDependencyValidator{}
		err := depValidator.ValidateDependency(ctx)
		if err != nil {
			return nil, fmt.Errorf("validating cargo dependency: %w", err)
		}

		return [][]string{{"cargo", "build", "--target", "wasm32-unknown-unknown", "--release"}}, nil
	default:
		return nil, fmt.Errorf("unsupported binary type %q", binaryType)
	}
}

func (b *BinaryBuilder) Build(ctx context.Context) error {
	if b.manifInfo.Manifest.Binaries == nil {
		fmt.Printf("⚠️  No binaries configured for build\n")
		return nil
	}

	if !b.isBuildRequired() {
		fmt.Printf("⚠️  All modules use external binaries, skipping build\n")
		return nil
	}

	found := false
	for binName, binary := range b.manifInfo.Manifest.Binaries {
		if binName != b.binaryLabel {
			continue
		}
		found = true

		var cmds [][]string
		var err error
		if binary.Build != "" {
			c := strings.Split(binary.Build, " ")
			cmds = [][]string{c}
		} else {
			cmds, err = b.getCmdArgs(ctx, binary.Type)
			if err != nil {
				return fmt.Errorf("getting build command for binary %s: %w", binName, err)
			}
		}

		for _, cmdArgs := range cmds {
			err = runCommandInDir(ctx, filepath.Dir(b.manifInfo.Path), cmdArgs)
			if err != nil {
				return fmt.Errorf("error running `%s`: %w", strings.Join(cmdArgs, " "), err)
			}
		}
	}

	if !found {
		return fmt.Errorf("binary label %q not found in manifest", b.binaryLabel)
	}

	fmt.Printf("🔧 Binary compilation complete\n")
	return nil
}

type DependencyValidator interface {
	ValidateDependency(ctx context.Context) error
}

type TinyGoDependencyValidator struct{}

func (t *TinyGoDependencyValidator) ValidateDependency(ctx context.Context) error {
	//run tinygo version on the machine.  error if exit code not 0
	fmt.Printf("🔍 Checking for TinyGo...\n")
	cmd := exec.CommandContext(ctx, "tinygo", "version")
	cmd.Env = os.Environ()
	err := cmd.Run()
	if err != nil {
		return fmt.Errorf(`TinyGo not found: %w\n
Install TinyGo from https://tinygo.org/getting-started/install`, err)
	}

	fmt.Printf("✅ TinyGo found\n")
	return nil
}

type CargoDependencyValidator struct{}

func (c *CargoDependencyValidator) ValidateDependency(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "cargo", "--version")
	cmd.Env = os.Environ()
	err := cmd.Run()
	if err != nil {
		return fmt.Errorf(`Rust/Cargo not found: %w\n
Install Rust from https://rustup.rs/
On Linux and macOS: curl https://sh.rustup.rs -sSf | sh`, err)
	}

	fmt.Printf("✅ Rust/Cargo found\n")
	return nil
}

type SPKGPacker struct {
	manifInfo *manifestInfo
}

func newSPKGPacker(manifInfo *manifestInfo) (*SPKGPacker, error) {
	return &SPKGPacker{
		manifInfo: manifInfo,
	}, nil
}

func (s *SPKGPacker) Build(ctx context.Context) error {
	defaultCmd := []string{"substreams", "pack", s.manifInfo.Path}
	err := runCommandInDir(ctx, filepath.Dir(s.manifInfo.Path), defaultCmd)
	if err != nil {
		return fmt.Errorf("error running pack: %w", err)
	}

	return nil
}

// findManifest searches for the substreams.yaml file starting from the current directory
// and moving up to the parent directories until it finds the file or reaches the user's $HOME directory.
func findManifest() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("error getting user home directory: %w", err)
	}

	originalDir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("error getting current directory: %w", err)
	}

	currentDir := originalDir
	for {
		manifestPath := filepath.Join(currentDir, "substreams.yaml")
		if _, err := os.Stat(manifestPath); err == nil {
			return manifestPath, nil
		}

		if currentDir == homeDir {
			break
		}

		parentDir := filepath.Dir(currentDir)
		if parentDir == currentDir {
			break
		}
		currentDir = parentDir
	}

	return "", fmt.Errorf("substreams.yaml file not found anywhere in directory path from %s to %s", originalDir, homeDir)
}

// runCommandInDir runs a command in the specified directory.
func runCommandInDir(ctx context.Context, dir string, cmdArgs []string) error {
	cmd := exec.CommandContext(ctx, cmdArgs[0], cmdArgs[1:]...)
	cmd.Env = append(os.Environ(), "CARGO_TERM_COLOR=always")
	cmd.Env = append(cmd.Env, "CLICOLOR_FORCE=true")
	cmd.Env = append(cmd.Env, "__SUBSTREAMS_INTERNAL_BUILD_INVOCATION__=true")
	cmd.Dir = dir

	// For better output synchronization, redirect directly to stdout/stderr
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Remove verbose command logging for cleaner output
	err := cmd.Start()
	if err != nil {
		return fmt.Errorf("error starting `%s`: %w", strings.Join(cmdArgs, " "), err)
	}

	err = cmd.Wait()
	if err != nil {
		return fmt.Errorf("error running `%s`: %w", strings.Join(cmdArgs, " "), err)
	}

	if cmd.ProcessState.ExitCode() != 0 {
		return fmt.Errorf("command %q failed with exit code %d", strings.Join(cmdArgs, " "), cmd.ProcessState.ExitCode())
	}

	return nil
}
