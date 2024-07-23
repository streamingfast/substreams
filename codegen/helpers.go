package codegen

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/streamingfast/substreams/manifest"

	"github.com/charmbracelet/huh"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"google.golang.org/protobuf/types/descriptorpb"
)

const outputTypeSQL = "sql"
const outputTypeSubgraph = "subgraph"

func getModule(pkg *pbsubstreams.Package, moduleName string) (*pbsubstreams.Module, error) {
	existingModules := pkg.GetModules().GetModules()
	for _, module := range existingModules {
		if (module.Name) == moduleName {
			return module, nil
		}
	}

	return nil, fmt.Errorf("module %q does not exists", moduleName)
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

func getExistingProtoTypes(protoFiles []*descriptorpb.FileDescriptorProto) map[string]*descriptorpb.DescriptorProto {
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

func buildGenerateCommandFromArgs(manifestPath, moduleName, networkName, outputType string, withDevEnv bool) error {
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

	if outputType == outputTypeSQL {
		if requestedModule.Output.Type != "proto:sf.substreams.sink.database.v1.DatabaseChanges" {
			return fmt.Errorf("requested module shoud have proto:sf.substreams.sink.database.v1.DatabaseChanges as output type")
		}
	}

	if pkg.GetPackageMeta()[0] == nil {
		return fmt.Errorf("package meta not found")
	}

	projectName := pkg.GetPackageMeta()[0].Name

	messageDescriptor, err := searchForMessageTypeIntoPackage(pkg, requestedModule.Output.Type)
	if err != nil {
		return fmt.Errorf("searching for message type: %w", err)
	}

	protoTypeMapping := getExistingProtoTypes(pkg.ProtoFiles)
	project := NewProject(projectName, networkName, requestedModule, messageDescriptor, protoTypeMapping)

	// Create an example entity from the output descriptor
	project.BuildExampleEntity()

	projectFiles, err := project.Render(outputType, withDevEnv)
	if err != nil {
		return fmt.Errorf("rendering project files: %w", err)
	}

	saveDir, err := createSaveDirForm()
	if err != nil {
		fmt.Println("creating save directory: %w", err)
	}

	err = saveProjectFiles(projectFiles, saveDir)
	if err != nil {
		fmt.Println("saving project files: %w", err)
	}

	return nil
}

func createSaveDirForm() (string, error) {
	saveDir := "output_sps"
	if cwd, err := os.Getwd(); err == nil {
		saveDir = filepath.Join(cwd, saveDir)
	}

	inputField := huh.NewInput().Title("In which directory do you want to generate the project?").Value(&saveDir)
	var WITH_ACCESSIBLE = false

	err := huh.NewForm(huh.NewGroup(inputField)).WithTheme(huh.ThemeCharm()).WithAccessible(WITH_ACCESSIBLE).Run()
	if err != nil {
		return "", fmt.Errorf("failed taking input: %w", err)
	}

	return saveDir, nil
}

func saveProjectFiles(projectFiles map[string][]byte, saveDir string) error {
	err := os.MkdirAll(saveDir, 0755)
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
