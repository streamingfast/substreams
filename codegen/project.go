package codegen

import (
	"bytes"
	"embed"
	"fmt"
	"io/fs"
	"strings"
	"text/template"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/golang-cz/textcase"
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
	EntityInfo       EntityInfo
	SpkgProjectName  string
	ManifestPath     string
	OutputType       OutputType
	Flavor           string
}

func (p *Project) AddSubgraphEntityType(name string, field *descriptorpb.FieldDescriptorProto) {
	subgraphtype := func() *SubgraphType {
		switch field.GetType() {
		case
			descriptorpb.FieldDescriptorProto_TYPE_UINT64,
			descriptorpb.FieldDescriptorProto_TYPE_FIXED64,
			descriptorpb.FieldDescriptorProto_TYPE_FIXED32,
			descriptorpb.FieldDescriptorProto_TYPE_UINT32,
			descriptorpb.FieldDescriptorProto_TYPE_SFIXED32,
			descriptorpb.FieldDescriptorProto_TYPE_SINT32,
			descriptorpb.FieldDescriptorProto_TYPE_SINT64,
			descriptorpb.FieldDescriptorProto_TYPE_INT64,
			descriptorpb.FieldDescriptorProto_TYPE_SFIXED64:
			return &SubgraphBigInt
		case descriptorpb.FieldDescriptorProto_TYPE_FLOAT, descriptorpb.FieldDescriptorProto_TYPE_DOUBLE:
			return &SubgraphBigDecimal
		case descriptorpb.FieldDescriptorProto_TYPE_INT32:
			return &SubgraphInt
		case descriptorpb.FieldDescriptorProto_TYPE_BOOL:
			return &SubgraphBoolean
		case descriptorpb.FieldDescriptorProto_TYPE_STRING:
			return &SubgraphString
		case descriptorpb.FieldDescriptorProto_TYPE_MESSAGE,
			descriptorpb.FieldDescriptorProto_TYPE_GROUP,
			descriptorpb.FieldDescriptorProto_TYPE_ENUM:
			// Let's not support the nested message and groups for now as it is more complex
			// and would probably require foreign tables / subgraph entities to work
			// not even sure this works as of today
			fmt.Printf("skipping message, group and enum - not supported for the moment ")
			return nil
		case descriptorpb.FieldDescriptorProto_TYPE_BYTES:
			return &SubgraphBytes
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
		default:
			panic("field type not supported")
		}
	}()

	if subgraphtype != nil {
		p.EntityTypes = append(p.EntityTypes, &SubgraphEntityType{name, *subgraphtype})
	}
}

func (p *Project) AddPostgreSQL(name string, field *descriptorpb.FieldDescriptorProto) {
	postgreSqltype := func() *SQLType {
		switch field.GetType() {
		case
			descriptorpb.FieldDescriptorProto_TYPE_UINT64,
			descriptorpb.FieldDescriptorProto_TYPE_FIXED64,
			descriptorpb.FieldDescriptorProto_TYPE_FIXED32,
			descriptorpb.FieldDescriptorProto_TYPE_UINT32,
			descriptorpb.FieldDescriptorProto_TYPE_SFIXED32,
			descriptorpb.FieldDescriptorProto_TYPE_SINT32,
			descriptorpb.FieldDescriptorProto_TYPE_SINT64,
			descriptorpb.FieldDescriptorProto_TYPE_INT64,
			descriptorpb.FieldDescriptorProto_TYPE_SFIXED64,
			descriptorpb.FieldDescriptorProto_TYPE_INT32:
			return &PostgresSqlInt
		case descriptorpb.FieldDescriptorProto_TYPE_FLOAT, descriptorpb.FieldDescriptorProto_TYPE_DOUBLE:
			return &PostgresSqlDecimal
		case descriptorpb.FieldDescriptorProto_TYPE_BOOL:
			return &PostgresSqlBoolean
		case descriptorpb.FieldDescriptorProto_TYPE_STRING:
			return &PostgresSqlText
		case descriptorpb.FieldDescriptorProto_TYPE_MESSAGE,
			descriptorpb.FieldDescriptorProto_TYPE_GROUP,
			descriptorpb.FieldDescriptorProto_TYPE_ENUM:
			// Let's not support the nested message and groups for now as it is more complex
			// and would probably require foreign tables / subgraph entities to work
			// not even sure this works as of today
			fmt.Printf("skipping message, group and enum - not supported for the moment ")
			return nil
		case descriptorpb.FieldDescriptorProto_TYPE_BYTES:
			return &PostgresSqlBytes
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
		default:
			fmt.Printf("unsupported type %s", field.GetType())
			return nil
		}
	}()

	if postgreSqltype != nil {
		p.EntityTypes = append(p.EntityTypes, &SQLEntityType{name, *postgreSqltype})
	}
}

func (p *Project) AddClickHouseSQLType(name string, field *descriptorpb.FieldDescriptorProto) {
	clickhouseSqlType := func() *SQLType {
		switch field.GetType() {
		case descriptorpb.FieldDescriptorProto_TYPE_BOOL:
			return &ClickhouseBoolean
		case descriptorpb.FieldDescriptorProto_TYPE_STRING:
			return &ClickhouseString
		case descriptorpb.FieldDescriptorProto_TYPE_BYTES:
			return &ClickhouseString
		case descriptorpb.FieldDescriptorProto_TYPE_FLOAT, descriptorpb.FieldDescriptorProto_TYPE_DOUBLE:
			return &ClickhouseDecimal
		case descriptorpb.FieldDescriptorProto_TYPE_UINT32:
			return &ClickhouseUInt32
		case descriptorpb.FieldDescriptorProto_TYPE_SFIXED32, descriptorpb.FieldDescriptorProto_TYPE_INT32, descriptorpb.FieldDescriptorProto_TYPE_SINT32, descriptorpb.FieldDescriptorProto_TYPE_FIXED32:
			return &ClickhouseInt32
		case descriptorpb.FieldDescriptorProto_TYPE_UINT64:
			return &ClickhouseUInt64
		case descriptorpb.FieldDescriptorProto_TYPE_SFIXED64, descriptorpb.FieldDescriptorProto_TYPE_FIXED64, descriptorpb.FieldDescriptorProto_TYPE_INT64, descriptorpb.FieldDescriptorProto_TYPE_SINT64:
			return &ClickhouseInt64

		case descriptorpb.FieldDescriptorProto_TYPE_MESSAGE,
			descriptorpb.FieldDescriptorProto_TYPE_GROUP,
			descriptorpb.FieldDescriptorProto_TYPE_ENUM:
			// Let's not support the nested message and groups for now as it is more complex
			// and would probably require foreign tables / subgraph entities to work
			// not even sure this works as of today
			fmt.Printf("skipping message, group and enum - not supported for the moment ")
			return nil
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
		default:
			fmt.Printf("unsupported type %s", field.GetType())
			return nil
		}
	}()

	if clickhouseSqlType != nil {
		p.EntityTypes = append(p.EntityTypes, &SQLEntityType{name, *clickhouseSqlType})
	}
}

func NewProject(
	name string,
	spkgProjectName string,
	network, manifestPath string,
	module *pbsubstreams.Module,
	outputDescriptor *descriptorpb.DescriptorProto,
	protoTypeMapping map[string]*descriptorpb.DescriptorProto,
	outputType OutputType,
	flavor string,
) *Project {
	return &Project{
		Network:          network,
		Name:             name,
		Module:           module,
		OutputDescriptor: outputDescriptor,
		EntityTypes:      []EntityType{},
		EntityInfo:       EntityInfo{},
		protoTypeMapping: protoTypeMapping,
		SpkgProjectName:  spkgProjectName,
		ManifestPath:     manifestPath,
		OutputType:       outputType,
		Flavor:           flavor,
	}
}

func (p *Project) BuildOutputEntity() error {
	for _, field := range p.OutputDescriptor.Field {
		if strings.ToLower(field.GetName()) == "id" {
			p.EntityInfo.HasAnID = true
			p.EntityInfo.IDFieldName = field.GetName()
		}

		var entityType EntityType
		if p.OutputType == "Subgraph" {
			p.AddSubgraphEntityType(field.GetName(), field)
			p.EntityTypes = append(p.EntityTypes, entityType)
		}

		if p.OutputType == "sql" {

		}

	}
	return nil
}

// func (p *Project) ExampleEntityHasID() bool {
// 	return p.Entity.ID != ""
// }

func (p *Project) HasExampleEntity() bool {
	return len(p.EntityTypes) > 0
}

func (p *Project) SQLImportVersion() string            { return "1.0.7" }
func (p *Project) DatabaseChangeImportVersion() string { return "1.2.1" }

func (p *Project) SubstreamsKebabName() string {
	return strings.ReplaceAll(p.Name, "_", "-")
}

func (p *Project) GetModuleName() string {
	return p.Module.Name
}

func (p *Project) SpkgNameWithoutExt() string {
	return strings.TrimSuffix(p.SpkgProjectName, ".spkg")
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

func (p *Project) SQLExtensionName() string {
	return textcase.SnakeCase(p.Name + "_sql")
}

func (p *Project) GetOutputProtoPath() string {
	return p.Module.Output.Type[strings.LastIndex(p.Module.Output.Type, ":")+1:]
}

func (p *Project) GetOutputProtobufPath() string {
	protobufPath := strings.ReplaceAll(p.GetOutputProtoPath(), ".", "/")
	return protobufPath
}

func (p *Project) GetRustOutputProtobufPath() string {
	splitPath := strings.Split(p.GetOutputProtoPath(), ".")

	splitPath[len(splitPath)-1] = toProtoPascalCase(splitPath[len(splitPath)-1])

	return strings.Join(splitPath, "::")
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
		"toLower":      strings.ToLower,
		"toCamelCase":  textcase.CamelCase,
		"toKebabCase":  textcase.KebabCase,
		"toPascalCase": toProtoPascalCase,
	}

	tpls, err := ParseFS(funcMap, templatesFS, "**/*.gotmpl")
	if err != nil {
		return nil, fmt.Errorf("parse templates: %w", err)
	}

	var templateFiles map[string]string
	switch p.OutputType {
	case Subgraph:
		templateFiles = map[string]string{
			"subgraph/buf.gen.yaml":           "buf.gen.yaml",
			"subgraph/package.json.gotmpl":    "package.json",
			"subgraph/tsconfig.json":          "tsconfig.json",
			"subgraph/subgraph.yaml.gotmpl":   "subgraph.yaml",
			"subgraph/schema.graphql.gotmpl":  "schema.graphql",
			"subgraph/src/mappings.ts.gotmpl": "src/mappings.ts",
			"subgraph/README.md":              "README.md",
		}

		if withDevEnv {
			templateFiles["subgraph/run-local.sh.gotmpl"] = "run-local.sh"
			templateFiles["subgraph/dev-environment/docker-compose.yml"] = "dev-environment/docker-compose.yml"
			templateFiles["subgraph/dev-environment/start.sh"] = "dev-environment/start.sh"
			templateFiles["subgraph/dev-environment/config.toml.gotmpl"] = "dev-environment/config.toml"
		}
	case Sql:
		templateFiles = map[string]string{
			"sql/src/lib.rs.gotmpl":   "src/lib.rs",
			"sql/Cargo.toml.gotmpl":   "Cargo.toml",
			"sql/README.md":           "README.md",
			"sql/rust-toolchain.toml": "rust-toolchain.toml",
		}

		switch p.Flavor {
		case "PostgresSQL":
			templateFiles["sql/schema.sql.gotmpl"] = "schema.sql"
			templateFiles["sql/substreams.sql.yaml.gotmpl"] = "substreams.yaml"
		case "ClickHouse":
			templateFiles["sql/schema.clickhouse.sql.gotmpl"] = "schema.sql"
			templateFiles["sql/substreams.clickhouse.yaml.gotmpl"] = "substreams.yaml"
		default:
			panic("flavor not supported")
		}

		if withDevEnv {
			panic("not implemented yet")
			//templateFiles["sql/run-local.sh.gotmpl"] = "run-local.sh"
			//templateFiles["sql/dev-environment/docker-compose.yml"] = "dev-environment/docker-compose.yml"
			//templateFiles["sql/dev-environment/start.sh"] = "dev-environment/start.sh"
			//templateFiles["sql/dev-environment/config.toml.gotmpl"] = "dev-environment/config.toml"
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
