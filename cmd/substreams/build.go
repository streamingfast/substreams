package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/streamingfast/cli"
	"github.com/streamingfast/cli/sflags"
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
	buildCmd.Flags().String("manifest", "", "Path to the manifest file")
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
			return fmt.Errorf("error finding manifest: %w", err)
		}
	}
	fmt.Printf("Building manifest file: %s\n", manifestPath)

	manif, err := readManifestYaml(manifestPath)
	if err != nil {
		return fmt.Errorf("error reading manifest: %w", err)
	}

	info := &manifestInfo{
		Path:     manifestPath,
		Manifest: manif,
	}

	binaryLabel := sflags.MustGetString(cmd, "binary")

	protoBuilder, err := newProtoBuilder(info, binaryLabel)
	if err != nil {
		return fmt.Errorf("error creating proto builder: %w", err)
	}
	err = protoBuilder.Build(ctx)
	if err != nil {
		return fmt.Errorf("error running protogen: %w", err)
	}

	binaryBuilder, err := newBinaryBuilder(info, binaryLabel)
	if err != nil {
		return fmt.Errorf("error creating binary builder: %w", err)
	}

	err = binaryBuilder.Build(ctx)
	if err != nil {
		return fmt.Errorf("error building binary: %w", err)
	}

	noPack := sflags.MustGetBool(cmd, "no-pack")
	if noPack {
		fmt.Printf("--no-pack flag detected, skipping creation of .spkg file.\n")
		fmt.Printf("Build complete.\n")
		return nil
	}

	spkgBuilder, err := newSPKGPacker(info)
	if err != nil {
		return fmt.Errorf("error creating spkg builder: %w", err)
	}

	err = spkgBuilder.Build(ctx)
	if err != nil {
		return fmt.Errorf("error building spkg: %w", err)
	}

	fmt.Printf("Build complete.\n")
	return nil
}

func readManifestYaml(manifestPath string) (manifest.Manifest, error) {
	var out *manifest.Manifest

	cnt, err := os.ReadFile(manifestPath)
	if err != nil {
		return manifest.Manifest{}, fmt.Errorf("error reading substreams manifest %q: %w", manifestPath, err)
	}

	decoder := yaml.NewDecoder(bytes.NewReader(cnt))
	decoder.KnownFields(true)
	if err := decoder.Decode(&out); err != nil {
		return manifest.Manifest{}, fmt.Errorf("error decoding manifest content: %w", err)
	}

	return *out, nil
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
		fmt.Println("Notice: No binaries found of type `wasm/rust-v1`, not generating rust bindings...")
		return nil
	}

	excludes := strings.Join(p.manifInfo.Manifest.Protobuf.ExcludePaths, ",")
	if excludes == "" {
		fmt.Printf("Notice: No exclude paths found:\n")
		fmt.Printf("* Typically, `google` and `sf/substreams` are excluded. If build fails, consider adding these exclude paths.\n")
	}

	defaultCmd := []string{"substreams", "protogen", p.manifInfo.Path}
	if excludes != "" {
		defaultCmd = append(defaultCmd, []string{"--exclude-paths", excludes}...)
	}

	err := runCommandInDir(ctx, filepath.Dir(p.manifInfo.Path), defaultCmd)
	if err != nil {
		return fmt.Errorf("error running protogen: %w", err)
	}

	fmt.Printf("Protogen complete.\n")
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
		fmt.Printf("All modules have a 'use' field\n")
		buildRequired = false
	} else {
		buildRequired = true
	}

	return buildRequired
}

func (b *BinaryBuilder) getCmdArgs(ctx context.Context, binaryType string) ([][]string, error) {
	binaryTypeID, _ := manifest.SplitBinaryType(binaryType)

	switch binaryTypeID {
	case "wasip1/tinygo-v1":
		fmt.Printf("`wasip1/tinygo-v1` binary type found...\n")
		depValidator := &TinyGoDependencyValidator{}
		err := depValidator.ValidateDependency(ctx)
		if err != nil {
			return nil, fmt.Errorf("validating tinygo dependency: %w", err)
		}

		return [][]string{{"tinygo", "build", "-o", "main.wasm", "-target", "wasi", "-gc", "leaking", "-scheduler", "none", "."}}, nil
	case "wasm/rust-v1":
		fmt.Printf("`wasm/rust-v1` binary type found...\n")
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
	if b.manifInfo.Manifest.Binaries == nil || len(b.manifInfo.Manifest.Binaries) == 0 {
		fmt.Printf("No binaries to build\n")
		return nil
	}

	if !b.isBuildRequired() {
		fmt.Printf("No build required.\n")
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

	fmt.Printf("Binary build complete.\n")
	return nil
}

type DependencyValidator interface {
	ValidateDependency(ctx context.Context) error
}

type TinyGoDependencyValidator struct{}

func (t *TinyGoDependencyValidator) ValidateDependency(ctx context.Context) error {
	//run tinygo version on the machine.  error if exit code not 0
	fmt.Printf("Checking for tinygo on the system...\n")
	cmd := exec.CommandContext(ctx, "tinygo", "version")
	cmd.Env = os.Environ()
	err := cmd.Run()
	if err != nil {
		return fmt.Errorf(`error validating presence of tinygo on machine: %w\n
Consider installing tinygo from https://tinygo.org/getting-started/install\n`, err)
	}

	fmt.Printf("tinygo found on the system\n")
	return nil
}

type CargoDependencyValidator struct{}

func (c *CargoDependencyValidator) ValidateDependency(ctx context.Context) error {
	//run cargo version on the machine.  error if exit code not 0
	fmt.Printf("Checking for cargo on the system...\n")
	cmd := exec.CommandContext(ctx, "cargo", "--version")
	cmd.Env = os.Environ()
	err := cmd.Run()
	if err != nil {
		return fmt.Errorf(`error validating presence of rust cargo on machine: %w\n
Consider installing rustup from https://rustup.rs/
On Linux and macOS systems, this is done as follows:
"curl https://sh.rustup.rs -sSf | sh"`, err)
	}

	fmt.Printf("cargo found on the system\n")
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

	fmt.Printf("Pack complete.\n")
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
	cmd.Dir = dir

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("error creating stdout pipe: %w", err)
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("error creating stderr pipe: %w", err)
	}

	go func() {
		_, _ = io.Copy(os.Stdout, stdoutPipe)
	}()
	go func() {
		_, _ = io.Copy(os.Stderr, stderrPipe)
	}()

	fmt.Printf("Running command in %s: `%s`...\n", dir, strings.Join(cmdArgs, " "))
	err = cmd.Start()
	if err != nil {
		return fmt.Errorf("error starting `%s`: %w", strings.Join(cmdArgs, " "), err)
	}

	err = cmd.Wait()
	if err != nil {
		return fmt.Errorf("error running `%s`: %w", strings.Join(cmdArgs, " "), err)
	}

	return nil
}
