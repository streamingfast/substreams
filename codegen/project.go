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
	EntityTypes      []EntityType
	SpkgProjectName  string
	OutputType       OutputType
}

func (p *Project) AddSubgraphEntityType(name string, ttype SubgraphType) {
	p.EntityTypes = append(p.EntityTypes, &SubgraphEntityType{
		Name: name,
		Type: ttype,
	})
}

func (p *Project) AddSQLEntityType(name string, ttype SqlType) {
	p.EntityTypes = append(p.EntityTypes, &SQLEntityType{
		Name: name,
		Type: ttype,
	})
}

func NewProject(
	name string,
	spkgProjectName string,
	network string,
	module *pbsubstreams.Module,
	outputDescriptor *descriptorpb.DescriptorProto,
	protoTypeMapping map[string]*descriptorpb.DescriptorProto,
	outputType OutputType,
) *Project {
	return &Project{
		Network:          network,
		Name:             name,
		Module:           module,
		OutputDescriptor: outputDescriptor,
		EntityTypes:      []EntityType{},
		protoTypeMapping: protoTypeMapping,
		SpkgProjectName:  spkgProjectName,
		OutputType:       outputType,
	}
}

func (p *Project) BuildOutputEntity() error {
	for _, field := range p.OutputDescriptor.Field {
		name := textcase.CamelCase(field.GetName())
		switch field.GetType() {
		case descriptorpb.FieldDescriptorProto_TYPE_DOUBLE,
			descriptorpb.FieldDescriptorProto_TYPE_FLOAT,
			descriptorpb.FieldDescriptorProto_TYPE_INT64,
			descriptorpb.FieldDescriptorProto_TYPE_UINT64,
			descriptorpb.FieldDescriptorProto_TYPE_FIXED64,
			descriptorpb.FieldDescriptorProto_TYPE_FIXED32,
			descriptorpb.FieldDescriptorProto_TYPE_UINT32,
			descriptorpb.FieldDescriptorProto_TYPE_SFIXED32,
			descriptorpb.FieldDescriptorProto_TYPE_SFIXED64,
			descriptorpb.FieldDescriptorProto_TYPE_SINT32,
			descriptorpb.FieldDescriptorProto_TYPE_SINT64:

			if p.OutputType == Subgraph {
				p.AddSubgraphEntityType(name, SubgraphBigInt)
			}

		case descriptorpb.FieldDescriptorProto_TYPE_INT32:
			if p.OutputType == Subgraph {
				p.AddSubgraphEntityType(name, SubgraphInt)
			}

		case descriptorpb.FieldDescriptorProto_TYPE_BOOL:
			if p.OutputType == Subgraph {
				p.AddSubgraphEntityType(name, SubgraphBoolean)
			}

		case descriptorpb.FieldDescriptorProto_TYPE_STRING:
			if p.OutputType == Subgraph {
				p.AddSubgraphEntityType(name, SubgraphString)
			}

		case descriptorpb.FieldDescriptorProto_TYPE_MESSAGE,
			descriptorpb.FieldDescriptorProto_TYPE_GROUP,
			descriptorpb.FieldDescriptorProto_TYPE_ENUM:
			// Let's not support the nested message and groups for now as it is more complex
			// and would probably require foreign tables / subgraph entities to work
			// not even sure this works as of today
			fmt.Println("skipping message, group and enum - not supported for the moment")

		case descriptorpb.FieldDescriptorProto_TYPE_BYTES:
			if p.OutputType == Subgraph {
				p.AddSubgraphEntityType(name, SubgraphBytes)
			}

		}

		// if field.GetType() == descriptorpb.FieldDescriptorProto_TYPE_MESSAGE {
		// 	if *field.Label == descriptorpb.FieldDescriptorProto_LABEL_REPEATED {
		//		splitMessagePath := strings.Split(typeName, ".")
		//		name splitMessagePath[len(splitMessagePath)-1]

		// 		p.Entities = append(p.Entities, &Entity{
		// 			// NameAsProtoField: textcase.CamelCase(field.GetName()),
		// 			// NameAsEntity:     "My" + name,
		// 			Name:             name,
		// 		})

		// 		if p.protoTypeMapping[*field.TypeName] == nil {
		// 			return fmt.Errorf("nested message type: %q not found", *field.TypeName)
		// 		}

		// 		for _, nestedMessageField := range p.protoTypeMapping[*field.TypeName].Field {
		// 			switch *nestedMessageField.Type {
		// 			case descriptorpb.FieldDescriptorProto_TYPE_STRING, descriptorpb.FieldDescriptorProto_TYPE_INT64, descriptorpb.FieldDescriptorProto_TYPE_UINT64, descriptorpb.FieldDescriptorProto_TYPE_UINT32, descriptorpb.FieldDescriptorProto_TYPE_BYTES:
		// 				p.Entity.ID = textcase.CamelCase(nestedMessageField.GetName())
		// 			default:
		// 				continue
		// 			}
		// 		}
		// 	}
		// }
	}
	return nil
}

// func (p *Project) ExampleEntityHasID() bool {
// 	return p.Entity.ID != ""
// }

func (p *Project) HasExampleEntity() bool {
	return len(p.EntityTypes) > 0
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

func (p *Project) Render(withDevEnv bool) (projectFiles map[string][]byte, err error) {
	//TODO: currently, we only support the simple use case of minimal codegens
	// Need to update this and supporte more complicated use cases
	projectFiles = map[string][]byte{}

	funcMap := template.FuncMap{
		"toLower":     strings.ToLower,
		"toCamelCase": textcase.CamelCase,
		"toKebabCase": textcase.KebabCase,
	}

	tpls, err := ParseFS(funcMap, templatesFS, "**/*.gotmpl")
	if err != nil {
		return nil, fmt.Errorf("parse templates: %w", err)
	}

	var templateFiles map[string]string
	switch p.OutputType {
	case Subgraph:
		templateFiles = map[string]string{
			"triggers/buf.gen.yaml":           "buf.gen.yaml",
			"triggers/package.json.gotmpl":    "package.json",
			"triggers/tsconfig.json":          "tsconfig.json",
			"triggers/subgraph.yaml.gotmpl":   "subgraph.yaml",
			"triggers/schema.graphql.gotmpl":  "schema.graphql",
			"triggers/src/mappings.ts.gotmpl": "src/mappings.ts",
		}

		if withDevEnv {
			templateFiles["triggers/run-local.sh.gotmpl"] = "run-local.sh"
			templateFiles["triggers/dev-environment/docker-compose.yml"] = "dev-environment/docker-compose.yml"
			templateFiles["triggers/dev-environment/start.sh"] = "dev-environment/start.sh"
			templateFiles["triggers/dev-environment/config.toml.gotmpl"] = "dev-environment/config.toml"
		}
	case Sql:
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
