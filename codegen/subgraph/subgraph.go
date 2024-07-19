package subgraph

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/streamingfast/cli/sflags"
	"github.com/streamingfast/substreams/manifest"
)

var SubgraphCmd = &cobra.Command{
	Use:   "subgraph <manifest_url> <module_name> <network>",
	Short: "Generate subgraph dev environment from substreams manifest",
	Args:  cobra.ExactArgs(3),
	RunE:  generateSubgraphEnv,
}

func init() {
	SubgraphCmd.Flags().Bool("with-dev-env", false, "generate graph node dev environment")
}

func generateSubgraphEnv(cmd *cobra.Command, args []string) error {
	manifestPath := args[0]
	moduleName := args[1]
	networkName := args[2]
	reader, err := manifest.NewReader(manifestPath)
	if err != nil {
		return fmt.Errorf("manifest reader: %w", err)
	}

	withDevEnv := sflags.MustGetBool(cmd, "with-dev-env")

	pkg, _, err := reader.Read()
	if err != nil {
		return fmt.Errorf("read manifest %q: %w", manifestPath, err)
	}

	requestedModule, err := GetModule(pkg, moduleName)
	if err != nil {
		return fmt.Errorf("getting module: %w", err)
	}

	if pkg.GetPackageMeta()[0] == nil {
		return fmt.Errorf("package meta not found")
	}

	projectName := pkg.GetPackageMeta()[0].Name

	messageDescriptor, err := SearchForMessageTypeIntoPackage(pkg, requestedModule.Output.Type)
	if err != nil {
		return fmt.Errorf("searching for message type: %w", err)
	}

	protoTypeMapping := GetExistingProtoTypes(pkg.ProtoFiles)

	entitiesMapping, err := GetProjectEntities(messageDescriptor, protoTypeMapping)
	if err != nil {
		panic(fmt.Errorf("getting entities: %w", err))
	}

	project := NewProject(projectName, networkName, requestedModule, messageDescriptor, entitiesMapping)

	projectFiles, err := project.Render(withDevEnv)
	if err != nil {
		return fmt.Errorf("rendering project files: %w", err)
	}

	saveDir := "/tmp/testSubCmd2/"

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
