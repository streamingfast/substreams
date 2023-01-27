package codegen

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/streamingfast/substreams/manifest"
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
func (g *ProjectGenerator) GenerateProject() error {
	engine := &Engine{Manifest: &manifest.Manifest{}}
	utils["getEngine"] = engine.GetEngine
	directories := []string{
		".cargo",
		"proto",
		"src",
		filepath.Join("src", "abi"),
		filepath.Join("src", "pb"),
	}
	if _, err := os.Stat(g.srcPath); errors.Is(err, os.ErrNotExist) {
		fmt.Printf("Creating missing %q folder\n", g.srcPath)
		if err := os.MkdirAll(g.srcPath, os.ModePerm); err != nil {
			return fmt.Errorf("creating src directory %v: %w", g.srcPath, err)
		}
	}

	projectPath := filepath.Join(g.srcPath, g.ProjectName)
	if _, err := os.Stat(projectPath); errors.Is(err, os.ErrNotExist) {
		fmt.Printf("Creating missing %q folder\n", g.srcPath)
		if err := os.MkdirAll(projectPath, 0755); err != nil {
			return fmt.Errorf("creating missing %q folder: %w", projectPath, err)
		}
	}

	// generate template from ./templates
	tmpls, err := template.New("templates").Funcs(utils).ParseFS(templates, "*/*.gotmpl", "*/*/*.gotmpl", "*/*/*/*.gotmpl")
	if err != nil {
		return fmt.Errorf("instantiate template: %w", err)
	}

	// create directories
	for _, dir := range directories {
		dirPath := filepath.Join(projectPath, dir)
		if _, err := os.Stat(dirPath); errors.Is(err, os.ErrNotExist) {
			fmt.Printf("Creating missing %q folder\n", dirPath)
			if err := os.MkdirAll(dirPath, os.ModePerm); err != nil {
				return fmt.Errorf("creating directory %v: %w", dirPath, err)
			}
		} else {
			fmt.Println("src directory already exists, skipping")
		}
	}

	// create files
	err = fs.WalkDir(templates, "templates", func(path string, d fs.DirEntry, err error) error {
		if d.IsDir() ||
			d.Name() == "externs.gotmpl" ||
			d.Name() == "libGen.gotmpl" ||
			d.Name() == "pb_mod.gotmpl" ||
			d.Name() == "substreamsGen.gotmpl" ||
			d.Name() == "mod.gotmpl" {
			return nil
		}
		relativeEmbedPath := strings.TrimPrefix(path, "templates"+string(os.PathSeparator))

		// Change duplicate template filenames
		if d.Name() == "abimodfile.gotmpl" || d.Name() == "pb-modfile.gotmpl" {
			relativeEmbedPath = relativeEmbedPath[:len(relativeEmbedPath)-17] + "mod.gotmpl"
		}
		if d.Name() == "abierc721.gotmpl" {
			relativeEmbedPath = relativeEmbedPath[:len(relativeEmbedPath)-16] + "erc721.gotmpl"
		}

		// Change extensions from .gotmpl
		if d.Name() == "cargo.gotmpl" ||
			d.Name() == "config.gotmpl" ||
			d.Name() == "rust-toolchain.gotmpl" {
			relativeEmbedPath = strings.ReplaceAll(relativeEmbedPath, ".gotmpl", ".toml")
		}
		if d.Name() == "substreams.gotmpl" {
			relativeEmbedPath = strings.ReplaceAll(relativeEmbedPath, ".gotmpl", ".yaml")
		}
		if d.Name() == "makefile.gotmpl" {
			relativeEmbedPath = strings.ReplaceAll(relativeEmbedPath, ".gotmpl", "")
		}
		if relativeEmbedPath == "proto/erc721.gotmpl" {
			relativeEmbedPath = strings.ReplaceAll(relativeEmbedPath, ".gotmpl", ".proto")
		}
		if d.Name() == "makefile.gotmpl" {
			relativeEmbedPath = strings.ReplaceAll(relativeEmbedPath, ".gotmpl", "")
		}
		if d.Name() == "lib.gotmpl" ||
			strings.Contains(relativeEmbedPath, "src/pb") ||
			strings.Contains(relativeEmbedPath, "src/abi") {
			relativeEmbedPath = strings.ReplaceAll(relativeEmbedPath, ".gotmpl", ".rs")
		}

		err = generate(path, tmpls, d.Name(), g, filepath.Join(projectPath, relativeEmbedPath))
		if err != nil {
			return fmt.Errorf("generating file %s: %w", path, err)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("walking files: %w", err)
	}

	return nil
}
