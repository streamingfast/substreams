package codegen

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/golang-cz/textcase"

	"github.com/charmbracelet/huh"
	"github.com/streamingfast/substreams/manifest"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"google.golang.org/protobuf/types/descriptorpb"
)

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

func buildGenerateCommandFromArgs(manifestPath string, outputType OutputType, withDevEnv bool) error {
	reader, err := manifest.NewReader(manifestPath)
	if err != nil {
		return fmt.Errorf("manifest reader: %w", err)
	}

	pkgBundle, err := reader.Read()
	if err != nil {
		return fmt.Errorf("read manifest %q: %w", manifestPath, err)
	}

	if pkgBundle == nil {
		return fmt.Errorf("no package found")
	}

	pkg := pkgBundle.Package

	moduleNames := []string{}
	for _, module := range pkg.Modules.Modules {
		if strings.Contains(module.Name, ":") {
			continue
		}

		if module.ModuleKind() == pbsubstreams.ModuleKindBlockIndex || module.ModuleKind() == pbsubstreams.ModuleKindStore || module.Output == nil {
			continue
		}

		if outputType == Sql {
			if module.Output.Type == "proto:sf.substreams.sink.database.v1.DatabaseChanges" {
				input := fmt.Sprintf("A module `%s` has database changes as output type... That means you can directly sink data from it, to an SQL database, using `substreams-sink-sql` binary...\n\n", module.Name)
				printMardown(input)
				continueCmd, err := askContinueCmd()
				if err != nil {
					return fmt.Errorf("asking for continue command: %w", err)
				}

				if !continueCmd {
					return nil
				}
				continue
			}
		}

		moduleNames = append(moduleNames, module.Name)
	}

	selectedModule, err := createSelectForm(moduleNames, "Please select a mapper module to build the subgraph from:")
	if err != nil {
		return fmt.Errorf("creating request module form: %w", err)
	}

	requestedModule, err := getModule(pkg, selectedModule)
	if err != nil {
		return fmt.Errorf("getting module: %w", err)
	}

	var flavor string
	if outputType == Sql {
		flavor, err = createSelectForm([]string{"PostgresSQL", "ClickHouse"}, "Please select a SQL flavor:")
		if err != nil {
			return fmt.Errorf("creating sql flavor form: %w", err)
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

	currentNetwork := pkg.Network
	if currentNetwork == "" {
		labels := []string{}
		for label := range ChainConfigByID {
			labels = append(labels, label)
		}

		selectedNetwork, err := createSelectForm(labels, "Please select a network to build the subgraph from:")
		if err != nil {
			return fmt.Errorf("creating network form: %w", err)
		}

		currentNetwork = selectedNetwork
	}

	if manifestPath == "" {
		manifestPath = "../" + spkgProjectName
	}

	project := NewProject(projectName, spkgProjectName, currentNetwork, manifestPath, requestedModule, messageDescriptor, protoTypeMapping, outputType, flavor)

	err = project.BuildOutputEntity()
	if err != nil {
		return fmt.Errorf("building output entity: %w", err)
	}

	if outputType == Sql {
		fmt.Println("Rendering project files for Substreams Sink SQL...")
	} else {
		fmt.Println("Rendering project files for Substreams-powered-subgraph...")
	}

	projectFiles, err := project.Render(withDevEnv)
	if err != nil {
		return fmt.Errorf("rendering project files: %w", err)
	}

	saveDir := "subgraph"

	if outputType == Sql {
		saveDir = "sql"
	}

	if cwd, err := os.Getwd(); err == nil {
		saveDir = filepath.Join(cwd, saveDir)
	}

	_, err = os.Stat(saveDir)
	if !os.IsNotExist(err) {
		fmt.Printf("A %s directory is already existing...", saveDir)
		saveDir, err = createSaveDirForm(fmt.Sprintf("%s-2", saveDir))
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

		if strings.HasSuffix(fileName, ".sh") {
			err = os.WriteFile(filePath, fileContent, 0755)
			if err != nil {
				return fmt.Errorf("saving file %s: %w", filePath, err)
			}
			continue
		}

		err = os.WriteFile(filePath, fileContent, 0644)
		if err != nil {
			return fmt.Errorf("saving file %s: %w", filePath, err)
		}
	}

	return nil
}

func createSelectForm(labels []string, title string) (string, error) {
	if len(labels) == 0 {
		fmt.Println("No labels found...")
	}

	sort.Strings(labels)

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
		Title(title).
		Options(options...).
		Value(&selection)

	form := huh.NewForm(huh.NewGroup(selectField)).WithTheme(huh.ThemeCharm())
	err := form.Run()
	if err != nil {
		return "", fmt.Errorf("failed taking input: %w", err)
	}

	return selection, nil
}

func askContinueCmd() (bool, error) {
	var continueCmd bool
	inputField := huh.NewConfirm().
		Title(fmt.Sprintf("Do you still want to proceed?")).
		Affirmative("Yes").
		Negative("No").
		Value(&continueCmd)

	err := huh.NewForm(huh.NewGroup(inputField)).WithTheme(huh.ThemeCharm()).WithAccessible(false).Run()
	if err != nil {
		return false, fmt.Errorf("failed taking confirmation input: %w", err)
	}

	return continueCmd, nil
}

func toProtoPascalCase(input string) string {
	input = textcase.PascalCase(input)

	reg := regexp.MustCompile(`(\d+)([a-zA-Z])`)

	input = reg.ReplaceAllStringFunc(input, func(match string) string {
		return match[:len(match)-1] + strings.ToUpper(string(match[len(match)-1]))
	})

	return input
}

func printMardown(input string) {
	fmt.Println(input)
}
