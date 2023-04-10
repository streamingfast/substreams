package codegen

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/streamingfast/eth-go"
	"github.com/streamingfast/substreams/manifest"
)

type ProjectGenerator struct {
	srcPath         string
	ProjectContract eth.Address
	ProjectAbi      string
	ProjectEvents   []CodegenEvent
	ProtoEvents     []ProtoEvent
	RustEvents      []RustEvent

	ProjectName       string
	ProjectVersion    string
	ProjectVersionNum string
	RustVersion       string
	SubstreamsVersion string
}

func NewProjectGenerator(srcPath, projectName string, projectContract eth.Address, abi string, events []CodegenEvent) *ProjectGenerator {
	pj := &ProjectGenerator{
		ProjectAbi:      abi,
		srcPath:         srcPath,
		ProjectContract: projectContract,
		ProjectEvents:   events,

		ProjectName: projectName,
	}
	for i, event := range events {
		pj.ProtoEvents = append(pj.ProtoEvents, event.getProtoEvent(i+1, &event))
		pj.RustEvents = append(pj.RustEvents, event.getRustEvent(&event))
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
		"abi",
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

	// generate template from ./templates/init_template
	templateFiles, err := template.New("templates").Funcs(utils).ParseFS(templates, "*/*/*.gotmpl", "*/*/*/*.gotmpl", "*/*/*/*/*.gotmpl")
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
		}
	}

	// create files
	err = fs.WalkDir(templates, "templates", func(directory string, d fs.DirEntry, err error) error {
		if d.IsDir() ||
			d.Name() == "externs.gotmpl" ||
			d.Name() == "libGen.gotmpl" ||
			d.Name() == "pb_mod.gotmpl" ||
			d.Name() == "substreamsGen.gotmpl" ||
			d.Name() == "mod.gotmpl" {
			return nil
		}

		relativeEmbedPath := strings.TrimPrefix(directory, path.Join("templates", "generator")+string(os.PathSeparator))

		// Change duplicate template filenames
		if d.Name() == "abimodfile.rs.gotmpl" || d.Name() == "pbmodfile.rs.gotmpl" || d.Name() == "abierc721.rs.gotmpl" {
			relativeEmbedPath = strings.TrimSuffix(relativeEmbedPath, d.Name())
			relativeEmbedPath = filepath.Join(relativeEmbedPath, "mod.rs")
		}

		// Change extensions from .gotmpl
		relativeEmbedPath = strings.TrimSuffix(relativeEmbedPath, ".gotmpl")

		err = generate(directory, templateFiles, d.Name(), g, filepath.Join(projectPath, relativeEmbedPath))
		if err != nil {
			return fmt.Errorf("generating file %s: %w", directory, err)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("walking files: %w", err)
	}

	return nil
}
