package codegen

import (
	"bytes"
	"embed"
	"fmt"
	"io/fs"
	"strings"
	"text/template"

	"github.com/golang-cz/textcase"

	"github.com/bmatcuk/doublestar/v4"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"google.golang.org/protobuf/types/descriptorpb"
)

type Project struct {
	Name             string
	Network          string
	Module           *pbsubstreams.Module
	OutputDescriptor *descriptorpb.DescriptorProto
	protoTypeMapping map[string]*descriptorpb.DescriptorProto
	ExampleEntity    *ExampleEntity
}

type ExampleEntity struct {
	Name             string
	NameAsProtoField string
	NameAsEntity     string
	ID               string
}

func NewProject(name, network string, module *pbsubstreams.Module, outputDescriptor *descriptorpb.DescriptorProto, protoTypeMapping map[string]*descriptorpb.DescriptorProto) *Project {
	return &Project{
		Network:          network,
		Name:             name,
		Module:           module,
		OutputDescriptor: outputDescriptor,
		protoTypeMapping: protoTypeMapping,
	}
}

func (p *Project) BuildExampleEntity() error {
	for _, field := range p.OutputDescriptor.Field {
		if *field.Type == descriptorpb.FieldDescriptorProto_TYPE_MESSAGE {
			if *field.Label == descriptorpb.FieldDescriptorProto_LABEL_REPEATED {
				splitMessagePath := strings.Split(*field.TypeName, ".")
				name := splitMessagePath[len(splitMessagePath)-1]

				p.ExampleEntity = &ExampleEntity{
					NameAsProtoField: textcase.CamelCase(field.GetName()),
					NameAsEntity:     "My" + name,
					Name:             name,
				}

				if p.protoTypeMapping[*field.TypeName] == nil {
					return fmt.Errorf("nested message type: %q not found", *field.TypeName)
				}

				for _, nestedMessageField := range p.protoTypeMapping[*field.TypeName].Field {
					switch *nestedMessageField.Type {
					case descriptorpb.FieldDescriptorProto_TYPE_STRING, descriptorpb.FieldDescriptorProto_TYPE_INT64, descriptorpb.FieldDescriptorProto_TYPE_UINT64, descriptorpb.FieldDescriptorProto_TYPE_UINT32, descriptorpb.FieldDescriptorProto_TYPE_BYTES:
						p.ExampleEntity.ID = textcase.CamelCase(nestedMessageField.GetName())
					default:
						continue
					}
				}
			}
		}
	}
	return nil
}

func (p *Project) ExampleEntityHasID() bool {
	return p.ExampleEntity.ID != ""
}

func (p *Project) HasExampleEntity() bool {
	return p.ExampleEntity != nil

}

func (p *Project) SubstreamsKebabName() string {
	return strings.ReplaceAll(p.Name, "_", "-")
}

func (p *Project) GetModuleName() string {
	return p.Module.Name
}

func (p *Project) OutputName() string {
	return p.OutputDescriptor.GetName()
}

func (p *Project) ProtoOutputName() string {
	return "proto" + p.OutputDescriptor.GetName()
}

func (p *Project) ProtoOutputNameToSnake() string {
	return textcase.SnakeCase("proto" + p.OutputDescriptor.GetName())
}

func (p *Project) GetOutputProtoPath() string {
	return p.Module.Output.Type[strings.LastIndex(p.Module.Output.Type, ":")+1:]
}

func (p *Project) GetOutputProtobufPath() string {
	protobufPath := strings.ReplaceAll(p.GetOutputProtoPath(), ".", "/")
	return protobufPath
}

func (p *Project) ChainEndpoint() (string, error) {
	if ChainConfigByID[p.Network] == nil {
		return "", fmt.Errorf("network %q not found", p.Network)
	}
	return ChainConfigByID[p.Network].FirehoseEndpoint, nil
}

func (p *Project) Render(outputType string, withDevEnv bool) (projectFiles map[string][]byte, err error) {
	projectFiles = map[string][]byte{}

	funcMap := template.FuncMap{
		"arr": func(els ...any) []any {
			return els
		},
		"toLower":     strings.ToLower,
		"toCamelCase": textcase.CamelCase,
		"toKebabCase": textcase.KebabCase,
	}

	tpls, err := ParseFS(funcMap, templatesFS, "**/*.gotmpl")
	if err != nil {
		return nil, fmt.Errorf("parse templates: %w", err)
	}

	var templateFiles map[string]string
	switch outputType {
	case outputTypeSubgraph:
		templateFiles = map[string]string{
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
	case outputTypeSQL:
		templateFiles = map[string]string{
			"sql/Makefile.gotmpl": "Makefile",
		}
		if withDevEnv {
			templateFiles[""] = ""
		}
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
