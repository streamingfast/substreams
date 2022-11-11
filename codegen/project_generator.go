package codegen

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type ProjectGenerator struct {
	srcPath string

	ProjectName       string
	ProjectVersion    string
	RustVersion       string
	SubstreamsVersion string
}

var DefaultProjectVersion = "0.0.0"
var DefaultRustVersion = "1.60.0"
var DefaultSubstreamsVersion = "0.4.0"

type ProjectGeneratorOption func(*ProjectGenerator)

func WithProjectVersion(version string) ProjectGeneratorOption {
	return func(g *ProjectGenerator) {
		g.ProjectVersion = version
	}
}

func WithRustVersion(version string) ProjectGeneratorOption {
	return func(g *ProjectGenerator) {
		g.RustVersion = version
	}
}

func NewProjectGenerator(srcPath, projectName string, opts ...ProjectGeneratorOption) *ProjectGenerator {
	pj := &ProjectGenerator{
		srcPath:           srcPath,
		ProjectName:       projectName,
		ProjectVersion:    DefaultProjectVersion,
		RustVersion:       DefaultRustVersion,
		SubstreamsVersion: DefaultSubstreamsVersion,
	}

	for _, opt := range opts {
		opt(pj)
	}

	return pj
}

func (g *ProjectGenerator) GenerateProject() error {
	if _, err := os.Stat(g.srcPath); errors.Is(err, os.ErrNotExist) {
		fmt.Printf("Creating missing %q folder\n", g.srcPath)
		if err := os.MkdirAll(g.srcPath, os.ModePerm); err != nil {
			return fmt.Errorf("creating src directory %v: %w", g.srcPath, err)
		}
	}

	fullPath := filepath.Join(g.srcPath, g.ProjectName)
	if _, err := os.Stat(fullPath); errors.Is(err, os.ErrNotExist) {
		fmt.Printf("Creating missing %q folder\n", g.srcPath)
		if err := os.MkdirAll(fullPath, 0755); err != nil {
			return fmt.Errorf("creating missing %q folder: %w", g.srcPath, err)
		}
	}

	fmt.Printf("Generating files in %q\n", fullPath)

	srcDir := filepath.Join(fullPath, "src")
	if _, err := os.Stat(srcDir); errors.Is(err, os.ErrNotExist) {
		fmt.Printf("Creating missing %q folder\n", srcDir)
		if err := os.MkdirAll(srcDir, os.ModePerm); err != nil {
			return fmt.Errorf("creating src directory %v: %w", srcDir, err)
		}
	} else {
		fmt.Println("src directory already exists, skipping")
	}

	protoDir := filepath.Join(fullPath, "proto")
	if _, err := os.Stat(protoDir); errors.Is(err, os.ErrNotExist) {
		fmt.Printf("Creating missing %q folder\n", protoDir)
		if err := os.MkdirAll(protoDir, os.ModePerm); err != nil {
			return fmt.Errorf("creating proto directory %v: %w", protoDir, err)
		}
	} else {
		fmt.Println("proto directory already exists, skipping")
	}

	abisDir := filepath.Join(fullPath, "abis")
	if _, err := os.Stat(abisDir); errors.Is(err, os.ErrNotExist) {
		fmt.Printf("Creating missing %q folder\n", abisDir)
		if err := os.MkdirAll(abisDir, os.ModePerm); err != nil {
			return fmt.Errorf("creating abis directory %v: %w", abisDir, err)
		}
	} else {
		fmt.Println("abis directory already exists, skipping")
	}

	cargoTomlPath := filepath.Join(fullPath, "Cargo.toml")
	if _, err := os.Stat(cargoTomlPath); errors.Is(err, os.ErrNotExist) {
		if err := generate("Cargo.toml", tplCargoToml, g, cargoTomlPath); err != nil {
			return fmt.Errorf("generating Cargo.toml: %w", err)
		}
	} else {
		fmt.Println("Cargo.toml already exists, skipping")
	}

	buildshPath := filepath.Join(fullPath, "build.sh")
	if _, err := os.Stat(buildshPath); errors.Is(err, os.ErrNotExist) {
		if err := generate("build.sh", tplBuildSh, g, buildshPath); err != nil {
			return fmt.Errorf("generating build.sh: %w", err)
		}
	} else {
		fmt.Println("build.sh already exists, skipping")
	}

	// generate manifest file if it does not exist
	manifestPath := filepath.Join(fullPath, "substreams.yaml")
	if _, err := os.Stat(manifestPath); errors.Is(err, os.ErrNotExist) {
		if err := generate("substreams.yaml", tplManifestYaml, g, manifestPath); err != nil {
			return fmt.Errorf("generating substreams.yaml: %w", err)
		}
	} else {
		fmt.Println("substreams.yaml already exists, skipping")
	}

	rustToolchainPath := filepath.Join(fullPath, "rust-toolchain.toml")
	if _, err := os.Stat(rustToolchainPath); errors.Is(err, os.ErrNotExist) {
		if err := generate("substreams.yaml", tplRustToolchain, g, rustToolchainPath); err != nil {
			return fmt.Errorf("generating substreams.yaml: %w", err)
		}
	} else {
		fmt.Println("rust-toolchain.toml already exists, skipping")
	}

	return nil
}
