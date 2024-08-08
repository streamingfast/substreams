package codegen

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/streamingfast/substreams/manifest"
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
		currentName := parentName + "." + nestedMessage.GetName()
		protoTypeMapping[currentName] = nestedMessage
		processMessage(nestedMessage, currentName, protoTypeMapping)
	}
}

func buildGenerateCommandFromArgs(manifestPath, outputType string, withDevEnv bool) error {
	reader, err := manifest.NewReader(manifestPath)
	if err != nil {
		return fmt.Errorf("manifest reader: %w", err)
	}

	pkg, _, err := reader.Read()
	if err != nil {
		return fmt.Errorf("read manifest %q: %w", manifestPath, err)
	}

	moduleNames := []string{}
	for _, module := range pkg.Modules.Modules {
		if module.Output != nil {
			moduleNames = append(moduleNames, module.Name)
		}
	}

	selectedModule, err := createRequestModuleForm(moduleNames)
	if err != nil {
		return fmt.Errorf("creating request module form: %w", err)
	}

	requestedModule, err := getModule(pkg, selectedModule)
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

	spkgProjectName := fmt.Sprintf("%s-%s.spkg", strings.Replace(pkg.PackageMeta[0].Name, "_", "-", -1), pkg.PackageMeta[0].Version)
	_, err = os.Stat(spkgProjectName)

	if os.IsNotExist(err) {
		red := "\033[31m"
		reset := "\033[0m"

		fmt.Printf("%sThe substreams package %q does not exist, make sure to run `substreams pack` command first%s\n\n!", red, spkgProjectName, reset)
	}

	messageDescriptor, err := searchForMessageTypeIntoPackage(pkg, requestedModule.Output.Type)
	if err != nil {
		return fmt.Errorf("searching for message type: %w", err)
	}

	protoTypeMapping := getExistingProtoTypes(pkg.ProtoFiles)

	if pkg.Network == "" {
		return fmt.Errorf("network not found in your manifest file")
	}

	project := NewProject(projectName, spkgProjectName, pkg.Network, requestedModule, messageDescriptor, protoTypeMapping)

	// Create an example entity from the output descriptor
	project.BuildExampleEntity()

	fmt.Println("Rendering project files for Substreams-powered-subgraph...")

	projectFiles, err := project.Render(outputType, withDevEnv)
	if err != nil {
		return fmt.Errorf("rendering project files: %w", err)
	}

	saveDir := "subgraph"
	if cwd, err := os.Getwd(); err == nil {
		saveDir = filepath.Join(cwd, saveDir)
	}

	_, err = os.Stat("subgraph")
	if !os.IsNotExist(err) {
		fmt.Println("A subgraph directory is already existing...")
		saveDir, err = createSaveDirForm("subgraph-2")
		if err != nil {
			return fmt.Errorf("creating save dir form: %w", err)
		}
	}

	fmt.Println("Writing to directory:\n", saveDir)

	err = saveProjectFiles(projectFiles, saveDir)
	if err != nil {
		return fmt.Errorf("saving project files: %w", err)
	}

	return nil
}

func createSaveDirForm(saveDir string) (string, error) {
	inputField := huh.NewInput().Title("In which sub-directory do you want to generate the project?").Value(&saveDir)
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

func createRequestModuleForm(labels []string) (string, error) {
	if len(labels) == 0 {
		fmt.Println("No labels found...")
	}

	var options []huh.Option[string]
	optionsMap := make(map[string]string)
	for i := 0; i < len(labels); i++ {
		entry := huh.Option[string]{
			Key:   labels[i],
			Value: labels[i],
		}
		options = append(options, entry)
		optionsMap[entry.Value] = entry.Key
	}

	var selection string
	selectField := huh.NewSelect[string]().
		Title("Please select a mapper module to build the subgraph from:").
		Options(options...).
		Value(&selection)

	form := huh.NewForm(huh.NewGroup(selectField)).WithTheme(huh.ThemeCharm())
	err := form.Run()
	if err != nil {
		return "", fmt.Errorf("failed taking input: %w", err)
	}

	return selection, nil
}
