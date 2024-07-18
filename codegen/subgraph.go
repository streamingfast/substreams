package codegen

import (
	"bytes"
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/spf13/cobra"
	"github.com/streamingfast/substreams/manifest"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"google.golang.org/protobuf/types/descriptorpb"
)

var subgraphCmd = &cobra.Command{
	Use:   "subgraph <manifest_url> <module_name>",
	Short: "Generate subgraph dev environment from substreams manifest",
	Args:  cobra.ExactArgs(2),
	RunE:  generateSubgraphEnv,
}

func init() {
	subgraphCmd.Flags().String("substreams-api-token-envvar", "SUBSTREAMS_API_TOKEN", "name of variable containing Substreams Authentication token")
	Cmd.AddCommand(subgraphCmd)
}

type Project struct {
	Name             string
	Network          string
	Module           *pbsubstreams.Module
	OutputDescriptor *descriptorpb.DescriptorProto
	ProtoTypeMapping map[string]*descriptorpb.DescriptorProto
}

func NewProject(name, network string, module *pbsubstreams.Module, outputDescriptor *descriptorpb.DescriptorProto, protoTypeMapping map[string]*descriptorpb.DescriptorProto) *Project {
	return &Project{
		Network:          network,
		Name:             name,
		Module:           module,
		OutputDescriptor: outputDescriptor,
		ProtoTypeMapping: protoTypeMapping,
	}
}

func (p *Project) SubstreamsKebabName() string {
	return strings.ReplaceAll(p.Name, "_", "-")
}

func (p *Project) GetModuleName() string {
	return p.Module.Name
}

func (p *Project) GetEntityOutputName() string {
	return p.OutputDescriptor.GetName()
}

func GetExistingProtoTypes(protoFiles []*descriptorpb.FileDescriptorProto) map[string]*descriptorpb.DescriptorProto {
	var protoTypeMapping = map[string]*descriptorpb.DescriptorProto{}
	for _, protoFile := range protoFiles {
		packageName := protoFile.GetPackage()
		for _, message := range protoFile.MessageType {
			currentName := "." + packageName + "." + message.GetName()
			protoTypeMapping[currentName] = message
			processMessage(message, currentName, protoTypeMapping)
		}
	}

	return protoTypeMapping
}

func processMessage(message *descriptorpb.DescriptorProto, parentName string, protoTypeMapping map[string]*descriptorpb.DescriptorProto) {
	for _, nestedMessage := range message.NestedType {
		currentName := "." + parentName + "." + nestedMessage.GetName()
		protoTypeMapping[currentName] = nestedMessage
		processMessage(nestedMessage, currentName, protoTypeMapping)
	}
}

func (p *Project) GetEntities() (map[string]map[string]string, error) {
	var outputMap = map[string]map[string]string{}
	err := p.GetEntityFromMessage(p.OutputDescriptor, outputMap)
	if err != nil {
		return nil, fmt.Errorf("getting entities: %w", err)
	}

	return outputMap, nil
}

func (p *Project) GetEntityFromMessage(message *descriptorpb.DescriptorProto, inputMap map[string]map[string]string) error {
	var fieldMapping = map[string]string{}
	for _, field := range message.GetField() {
		switch *field.Type {
		case descriptorpb.FieldDescriptorProto_TYPE_BYTES:
			fieldMapping[field.GetName()] = "Bytes!"
		case descriptorpb.FieldDescriptorProto_TYPE_UINT64:
			fieldMapping[field.GetName()] = "Int!"
		case descriptorpb.FieldDescriptorProto_TYPE_INT64:
			fieldMapping[field.GetName()] = "Int!"
		case descriptorpb.FieldDescriptorProto_TYPE_INT32:
			fieldMapping[field.GetName()] = "Int!"
		case descriptorpb.FieldDescriptorProto_TYPE_UINT32:
			fieldMapping[field.GetName()] = "Int!"
		case descriptorpb.FieldDescriptorProto_TYPE_STRING:
			fieldMapping[field.GetName()] = "String!"
		case descriptorpb.FieldDescriptorProto_TYPE_MESSAGE:
			sanitizeFieldName := (*field.TypeName)[strings.LastIndex(*field.TypeName, ".")+1:]
			switch *field.Label {
			case descriptorpb.FieldDescriptorProto_LABEL_REPEATED:
				fieldMapping[field.GetName()] = "[" + sanitizeFieldName + "]!"
			case descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL:
				fieldMapping[field.GetName()] = sanitizeFieldName + "!"
			case descriptorpb.FieldDescriptorProto_LABEL_REQUIRED:
				fieldMapping[field.GetName()] = sanitizeFieldName + "!"
			default:
				return fmt.Errorf("field label %q not supported", *field.Label)
			}
			nestedMessage := p.ProtoTypeMapping[*field.TypeName]
			err := p.GetEntityFromMessage(nestedMessage, inputMap)
			if err != nil {
				return fmt.Errorf("getting entity from message: %w", err)
			}
		default:
			return fmt.Errorf("field type %q not supported", *field.Type)
		}
	}

	inputMap[message.GetName()] = fieldMapping
	return nil
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

func (p *Project) Render() (projectFiles map[string][]byte, err error) {
	projectFiles = map[string][]byte{}

	tpls, err := ParseFS(nil, templatesFS, "**/*.gotmpl")
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

func getModule(pkg *pbsubstreams.Package, moduleName string) (*pbsubstreams.Module, error) {
	existingModules := pkg.GetModules().GetModules()
	for _, module := range existingModules {
		if (module.Name) == moduleName {
			return module, nil
		}
	}

	return nil, fmt.Errorf("module %q does not exists", moduleName)
}

// delete all partial files which are already merged into the kv store
func generateSubgraphEnv(cmd *cobra.Command, args []string) error {
	//ctx := cmd.Context()
	manifestPath := args[0]
	moduleName := args[1]
	reader, err := manifest.NewReader(manifestPath)
	if err != nil {
		return fmt.Errorf("manifest reader: %w", err)
	}

	pkg, _, err := reader.Read()
	if err != nil {
		return fmt.Errorf("read manifest %q: %w", manifestPath, err)
	}

	requestedModule, err := getModule(pkg, moduleName)
	if err != nil {
		return fmt.Errorf("getting module: %w", err)
	}

	if pkg.GetPackageMeta()[0] == nil {
		return fmt.Errorf("package meta not found")
	}

	messageDescriptor, err := searchForMessageTypeIntoPackage(pkg, requestedModule.Output.Type)
	if err != nil {
		return fmt.Errorf("searching for message type: %w", err)
	}

	protoTypeMapping := GetExistingProtoTypes(pkg.ProtoFiles)

	project := NewProject(pkg.GetPackageMeta()[0].Name, pkg.Network, requestedModule, messageDescriptor, protoTypeMapping)

	projectFiles, err := project.Render()
	if err != nil {
		return fmt.Errorf("rendering project files: %w", err)
	}

	saveDir := "/tmp/testSubCmd/"

	err = os.MkdirAll(saveDir, 0755)
	if err != nil {
		return fmt.Errorf("creating directory %s: %w", saveDir, err)
	}

	for fileName, fileContent := range projectFiles {
		filePath := filepath.Join(saveDir, fileName)

		err := os.MkdirAll(filepath.Dir(filePath), 0755)
		if err != nil {
			return fmt.Errorf("creating directory %s: %w", filepath.Dir(filePath), err)
		}

		err = os.WriteFile(filePath, fileContent, 0644)
		if err != nil {
			return fmt.Errorf("saving file %s: %w", filePath, err)
		}
	}

	return nil
}

func searchForMessageTypeIntoPackage(pkg *pbsubstreams.Package, outputType string) (*descriptorpb.DescriptorProto, error) {
	sanitizeMessageType := outputType[strings.Index(outputType, ":")+1:]
	for _, protoFile := range pkg.ProtoFiles {
		packageName := protoFile.GetPackage()
		for _, message := range protoFile.MessageType {
			if packageName+"."+message.GetName() == sanitizeMessageType {
				return message, nil
			}

			nestedMessage := checkNestedMessages(message, packageName, sanitizeMessageType)
			if nestedMessage != nil {
				return nestedMessage, nil
			}
		}
	}

	return nil, fmt.Errorf("message type %q not found in package", sanitizeMessageType)
}

func checkNestedMessages(message *descriptorpb.DescriptorProto, packageName, messageType string) *descriptorpb.DescriptorProto {
	for _, nestedMessage := range message.NestedType {
		if packageName+"."+message.GetName()+"."+nestedMessage.GetName() == messageType {
			return nestedMessage
		}

		checkNestedMessages(nestedMessage, packageName, messageType)
	}

	return nil
}
