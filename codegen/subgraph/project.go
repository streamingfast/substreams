package subgraph

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
	"io/fs"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"google.golang.org/protobuf/types/descriptorpb"
)

type Project struct {
	Name             string
	Network          string
	Module           *pbsubstreams.Module
	OutputDescriptor *descriptorpb.DescriptorProto
	EntitiesMapping  map[string]Entity
}

func NewProject(name, network string, module *pbsubstreams.Module, outputDescriptor *descriptorpb.DescriptorProto, entitiesMapping map[string]Entity) *Project {
	return &Project{
		Network:          network,
		Name:             name,
		Module:           module,
		OutputDescriptor: outputDescriptor,
		EntitiesMapping:  entitiesMapping,
	}
}

func (p *Project) GetMainEntity() Entity {
	return p.EntitiesMapping[p.OutputDescriptor.GetName()]
}

func (p *Project) GetMainEntityName() string {
	return p.OutputDescriptor.GetName()
}

func (p *Project) SubstreamsKebabName() string {
	return strings.ReplaceAll(p.Name, "_", "-")
}

func (p *Project) GetModuleName() string {
	return p.Module.Name
}

func (p *Project) GetModuleOutputProtoPath() string {
	return p.Module.Output.Type[strings.LastIndex(p.Module.Output.Type, ":")+1:]
}

func (p *Project) GetModuleOutputProtobufPath() string {
	return strings.ReplaceAll(p.GetModuleOutputProtoPath(), ".", "/")
}

func (p *Project) GetEntities() map[string]Entity {
	return p.EntitiesMapping
}

func (p *Project) ChainEndpoint() string { return ChainConfigByID[p.Network].FirehoseEndpoint }

func (p *Project) Render(withDevEnv bool) (projectFiles map[string][]byte, err error) {
	projectFiles = map[string][]byte{}

	funcMap := template.FuncMap{
		"arr": func(els ...any) []any {
			return els
		},
	}

	tpls, err := ParseFS(funcMap, templatesFS, "**/*.gotmpl")
	if err != nil {
		return nil, fmt.Errorf("parse templates: %w", err)
	}

	templateFiles := map[string]string{
		"triggers/Makefile.gotmpl":        "Makefile",
		"triggers/buf.gen.yaml":           "buf.gen.yaml",
		"triggers/package.json.gotmpl":    "package.json",
		"triggers/tsconfig.json":          "tsconfig.json",
		"triggers/subgraph.yaml.gotmpl":   "subgraph.yaml",
		"triggers/schema.graphql.gotmpl":  "schema.graphql",
		"triggers/src/mappings.ts.gotmpl": "src/mappings.ts",
		"triggers/run-local.sh.gotmpl":    "run-local.sh",
	}

	if withDevEnv {
		templateFiles["triggers/dev-environment/docker-compose.yml"] = "dev-environment/docker-compose.yml"
		templateFiles["triggers/dev-environment/start.sh"] = "dev-environment/start.sh"
		templateFiles["triggers/dev-environment/config.toml.gotmpl"] = "dev-environment/config.toml"
	}

	for templateFile, finalFileName := range templateFiles {
		var content []byte
		if strings.HasSuffix(templateFile, ".gotmpl") {
			buffer := &bytes.Buffer{}
			if err := tpls.ExecuteTemplate(buffer, templateFile, p); err != nil {
				return nil, fmt.Errorf("embed render entry template %q: %w", templateFile, err)
			}
			content = buffer.Bytes()
		} else {
			content, err = templatesFS.ReadFile("templates/" + templateFile)
			if err != nil {
				return nil, fmt.Errorf("reading %q: %w", templateFile, err)
			}
		}

		projectFiles[finalFileName] = content
	}

	return
}

//go:embed templates/*
var templatesFS embed.FS

func ParseFS(myFuncs template.FuncMap, fsys fs.FS, pattern string) (*template.Template, error) {
	t := template.New("").Funcs(myFuncs)
	filenames, err := doublestar.Glob(fsys, pattern)
	if err != nil {
		return nil, err
	}
	if len(filenames) == 0 {
		return nil, fmt.Errorf("template: pattern matches no files: %#q", pattern)
	}

	for _, filename := range filenames {
		b, err := fs.ReadFile(fsys, filename)
		if err != nil {
			return nil, err
		}

		name, _ := strings.CutPrefix(filename, "templates/")

		_, err = t.New(name).Parse(string(b))
		if err != nil {
			return nil, err
		}
	}
	return t, nil
}
