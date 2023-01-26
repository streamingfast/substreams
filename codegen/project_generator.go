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
	ProjectVersionNum string
	RustVersion       string
	SubstreamsVersion string
}

var DefaultProjectVersion = "v0.0.1"

// DefaultProjectVersionNum for the manifest to produce 'v0.0.0' -> '0.0.0'
var DefaultProjectVersionNum = DefaultProjectVersion[1:]
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
		ProjectVersionNum: DefaultProjectVersionNum,
		RustVersion:       DefaultRustVersion,
		SubstreamsVersion: DefaultSubstreamsVersion,
	}

	for _, opt := range opts {
		opt(pj)
	}

	return pj
}
func (g *ProjectGenerator) GenerateProjectTest() error {
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

	fileEntries, err := templates.ReadDir("templates")
	if err != nil {
		return fmt.Errorf("reading all files from dir: %w", err)
	}

	for i, _ := range fileEntries {
		fmt.Printf("reading file: %s", fileEntries[i].Name())
	}
	return nil
}

//func (g *ProjectGenerator) GenerateProject() error {
//	if _, err := os.Stat(g.srcPath); errors.Is(err, os.ErrNotExist) {
//		fmt.Printf("Creating missing %q folder\n", g.srcPath)
//		if err := os.MkdirAll(g.srcPath, os.ModePerm); err != nil {
//			return fmt.Errorf("creating src directory %v: %w", g.srcPath, err)
//		}
//	}
//
//	fullPath := filepath.Join(g.srcPath, g.ProjectName)
//	if _, err := os.Stat(fullPath); errors.Is(err, os.ErrNotExist) {
//		fmt.Printf("Creating missing %q folder\n", g.srcPath)
//		if err := os.MkdirAll(fullPath, 0755); err != nil {
//			return fmt.Errorf("creating missing %q folder: %w", g.srcPath, err)
//		}
//	}
//
//	srcDir := filepath.Join(fullPath, "src")
//	if _, err := os.Stat(srcDir); errors.Is(err, os.ErrNotExist) {
//		fmt.Printf("Creating missing %q folder\n", srcDir)
//		if err := os.MkdirAll(srcDir, os.ModePerm); err != nil {
//			return fmt.Errorf("creating src directory %v: %w", srcDir, err)
//		}
//	} else {
//		fmt.Println("src directory already exists, skipping")
//	}
//
//	srcPbDir := filepath.Join(srcDir, "pb")
//	if _, err := os.Stat(srcPbDir); errors.Is(err, os.ErrNotExist) {
//		fmt.Printf("Creating missing %q folder\n", srcPbDir)
//		if err := os.MkdirAll(srcPbDir, os.ModePerm); err != nil {
//			return fmt.Errorf("creating src/pb directory %v: %w", srcPbDir, err)
//		}
//	} else {
//		fmt.Println("src/pb directory already exists, skipping")
//	}
//
//	srcAbiDir := filepath.Join(srcDir, "abi")
//	if _, err := os.Stat(srcAbiDir); errors.Is(err, os.ErrNotExist) {
//		fmt.Printf("Creating missing %q folder\n", srcAbiDir)
//		if err := os.MkdirAll(srcAbiDir, os.ModePerm); err != nil {
//			return fmt.Errorf("creating src/abi directory %v: %w", srcAbiDir, err)
//		}
//	} else {
//		fmt.Println("src/abi directory already exists, skipping")
//	}
//
//	protoDir := filepath.Join(fullPath, "proto")
//	if _, err := os.Stat(protoDir); errors.Is(err, os.ErrNotExist) {
//		fmt.Printf("Creating missing %q folder\n", protoDir)
//		if err := os.MkdirAll(protoDir, os.ModePerm); err != nil {
//			return fmt.Errorf("creating proto directory %v: %w", protoDir, err)
//		}
//	} else {
//		fmt.Println("proto directory already exists, skipping")
//	}
//
//	abiDir := filepath.Join(fullPath, "abi")
//	if _, err := os.Stat(abiDir); errors.Is(err, os.ErrNotExist) {
//		fmt.Printf("Creating missing %q folder\n", abiDir)
//		if err := os.MkdirAll(abiDir, os.ModePerm); err != nil {
//			return fmt.Errorf("creating abi directory %v: %w", abiDir, err)
//		}
//	} else {
//		fmt.Println("abi directory already exists, skipping")
//	}
//
//	cargoDir := filepath.Join(fullPath, ".cargo")
//	if _, err := os.Stat(cargoDir); errors.Is(err, os.ErrNotExist) {
//		fmt.Printf("Creating missing %q folder\n", cargoDir)
//		if err := os.MkdirAll(cargoDir, os.ModePerm); err != nil {
//			return fmt.Errorf("creating .cargo directory %v: %w", cargoDir, err)
//		}
//	} else {
//		fmt.Println(".cargo directory already exists, skipping")
//	}
//
//	abiModPath := filepath.Join(srcAbiDir, "mod.rs")
//	if _, err := os.Stat(abiModPath); errors.Is(err, os.ErrNotExist) {
//		if err := generate("mod.rs", tplAbiModFile, g, abiModPath); err != nil {
//			return fmt.Errorf("generating abi/mod.rs file: %w", err)
//		}
//	} else {
//		fmt.Println("abi/mod.rs already exists, skipping")
//	}
//
//	abiErcPath := filepath.Join(srcAbiDir, "erc721.rs")
//	if _, err := os.Stat(abiErcPath); errors.Is(err, os.ErrNotExist) {
//		if err := generate("erc721.rs", tplAbiErcFile, g, abiErcPath); err != nil {
//			return fmt.Errorf("generating abi/erc721.rs file: %w", err)
//		}
//	} else {
//		fmt.Println("abi/erc721.rs already exists, skipping")
//	}
//
//	pbModPath := filepath.Join(srcPbDir, "mod.rs")
//	if _, err := os.Stat(pbModPath); errors.Is(err, os.ErrNotExist) {
//		if err := generate("mod.rs", tplPbModFile, g, pbModPath); err != nil {
//			return fmt.Errorf("generating pb/mod.rs file: %w", err)
//		}
//	} else {
//		fmt.Println("pb/mod.rs already exists, skipping")
//	}
//
//	pbProtogenPath := filepath.Join(srcPbDir, "eth.erc721.v1.rs")
//	if _, err := os.Stat(pbProtogenPath); errors.Is(err, os.ErrNotExist) {
//		if err := generate("eth.erc721.v1.rs", tplProtogenFile, g, pbProtogenPath); err != nil {
//			return fmt.Errorf("generating eth.erc721.v1.rs file: %w", err)
//		}
//	} else {
//		fmt.Println("protogen already exists, skipping")
//	}
//
//	libFilePath := filepath.Join(srcDir, "lib.rs")
//	if _, err := os.Stat(libFilePath); errors.Is(err, os.ErrNotExist) {
//		if err := generate("erc721.proto", tplLibFile, g, libFilePath); err != nil {
//			return fmt.Errorf("generating lib.rs file: %w", err)
//		}
//	} else {
//		fmt.Println("proto definition already exists, skipping")
//	}
//
//	protoFilePath := filepath.Join(protoDir, "erc721.proto")
//	if _, err := os.Stat(protoFilePath); errors.Is(err, os.ErrNotExist) {
//		if err := generate("erc721.proto", tplProtoFile, g, protoFilePath); err != nil {
//			return fmt.Errorf("generating erc721.proto: %w", err)
//		}
//	} else {
//		fmt.Println("proto definition already exists, skipping")
//	}
//
//	cargoTomlPath := filepath.Join(fullPath, "Cargo.toml")
//	if _, err := os.Stat(cargoTomlPath); errors.Is(err, os.ErrNotExist) {
//		if err := generate("Cargo.toml", tplCargoToml, g, cargoTomlPath); err != nil {
//			return fmt.Errorf("generating Cargo.toml: %w", err)
//		}
//	} else {
//		fmt.Println("Cargo.toml already exists, skipping")
//	}
//
//	// generate makefile if it does not exist
//	makeFilePath := filepath.Join(fullPath, "Makefile")
//	if _, err := os.Stat(makeFilePath); errors.Is(err, os.ErrNotExist) {
//		if err := generate("makefile", tplMakefile, g, makeFilePath); err != nil {
//			return fmt.Errorf("generating makefile: %w", err)
//		}
//	} else {
//		fmt.Println("makefile already exists, skipping")
//	}
//
//	// generate manifest file if it does not exist
//	manifestPath := filepath.Join(fullPath, "substreams.yaml")
//	if _, err := os.Stat(manifestPath); errors.Is(err, os.ErrNotExist) {
//		if err := generate("substreams.yaml", tplManifestYaml, g, manifestPath); err != nil {
//			return fmt.Errorf("generating substreams.yaml: %w", err)
//		}
//	} else {
//		fmt.Println("substreams.yaml already exists, skipping")
//	}
//
//	rustToolchainPath := filepath.Join(fullPath, "rust-toolchain.toml")
//	if _, err := os.Stat(rustToolchainPath); errors.Is(err, os.ErrNotExist) {
//		if err := generate("substreams.yaml", tplRustToolchain, g, rustToolchainPath); err != nil {
//			return fmt.Errorf("generating substreams.yaml: %w", err)
//		}
//	} else {
//		fmt.Println("rust-toolchain.toml already exists, skipping")
//	}
//
//	cargoConfigPath := filepath.Join(cargoDir, "config.toml")
//	if _, err := os.Stat(cargoConfigPath); errors.Is(err, os.ErrNotExist) {
//		if err := generate("config.toml", tplCargoConfig, g, cargoConfigPath); err != nil {
//			return fmt.Errorf("generating .cargo/config.toml: %w", err)
//		}
//	} else {
//		fmt.Println("rust-toolchain.toml already exists, skipping")
//	}
//
//	return nil
//}
