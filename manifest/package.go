package manifest

import (
	"fmt"
	"github.com/jhump/protoreflect/desc"
	"github.com/jhump/protoreflect/dynamic"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"os"
	"path"
	"path/filepath"
)

type manifestConverter struct {
	inputPath string

	sinkConfigDynamicMessage       *dynamic.Message
	skipSourceCodeImportValidation bool
}

func newManifestConverter(inputPath string, skipSourceCodeImportValidation bool) *manifestConverter {
	return &manifestConverter{
		inputPath:                      inputPath,
		skipSourceCodeImportValidation: skipSourceCodeImportValidation,
	}
}

func (r *manifestConverter) Convert(manif *Manifest) (*pbsubstreams.Package, []*desc.FileDescriptor, *dynamic.Message, error) {

	if err := r.expandManifestVariables(manif); err != nil {
		return nil, nil, nil, err
	}

	if err := r.validateManifest(manif); err != nil {
		return nil, nil, nil, fmt.Errorf("unable to load manifest: %w", err)
	}

	return r.manifestToPkg(manif)
}

func (r *manifestConverter) expandManifestVariables(manif *Manifest) error {
	abs, err := filepath.Abs(r.inputPath)
	if err != nil {
		return fmt.Errorf("unable to get working dir: %w", err)
	}
	manif.Workdir = path.Dir(abs)
	// Allow environment variables in `imports` element
	for i, moduleImport := range manif.Imports {
		manif.Imports[i][1] = os.ExpandEnv(moduleImport[1])
	}

	// Allow environment variables in `protobuf.importPaths` element
	for i := range manif.Protobuf.ImportPaths {
		manif.Protobuf.ImportPaths[i] = os.ExpandEnv(manif.Protobuf.ImportPaths[i])
	}
	return nil
}

func (r *manifestConverter) validateManifest(manif *Manifest) error {

	if manif.SpecVersion != "v0.1.0" {
		return fmt.Errorf("invalid 'specVersion', must be v0.1.0")
	}

	// TODO: put some limits on the NUMBER of modules (max 50 ?)
	// TODO: put a limit on the SIZE of the WASM payload (max 10MB per binary?)

	for _, s := range manif.Modules {
		// TODO: let's make sure this is also checked when received in Protobuf in a remote request.

		switch s.Kind {
		case ModuleKindMap:
			if s.Output.Type == "" {
				return fmt.Errorf("stream %q: missing 'output.type' for kind 'map'", s.Name)
			}
		case ModuleKindStore:
			if err := validateStoreBuilder(s); err != nil {
				return fmt.Errorf("stream %q: %w", s.Name, err)
			}
		case "":
			if s.Use == "" {
				return fmt.Errorf("module kind not specified for %q", s.Name)
			}

			if err := validateModuleWithUse(s); err != nil {
				return fmt.Errorf("stream %q: %w", s.Name, err)
			}

		default:
			return fmt.Errorf("stream %q: invalid kind %q", s.Name, s.Kind)
		}

		for idx, input := range s.Inputs {
			if err := input.parse(); err != nil {
				return fmt.Errorf("module %q: invalid input [%d]: %w", s.Name, idx, err)
			}
		}
	}

	return nil
}

func handleUseModules(pkg *pbsubstreams.Package, manif *Manifest) error {
	packageModulesMapping := make(map[string]*pbsubstreams.Module)
	for _, module := range pkg.Modules.Modules {
		packageModulesMapping[module.Name] = module
	}

	for _, manifestModule := range manif.Modules {
		if manifestModule.Use == "" {
			continue
		}

		usedModule, found := packageModulesMapping[manifestModule.Use]
		if !found {
			return fmt.Errorf("module %q: use module %q not found", manifestModule.Name, manifestModule.Use)
		}
		moduleWithUse := packageModulesMapping[manifestModule.Name]

		if err := checkEqualInputs(moduleWithUse, usedModule, manifestModule, packageModulesMapping); err != nil {
			return fmt.Errorf("checking inputs for module %q: %w", manifestModule.Name, err)
		}

		moduleWithUse.BinaryIndex = usedModule.BinaryIndex
		moduleWithUse.BinaryEntrypoint = usedModule.BinaryEntrypoint

		if moduleWithUse.InitialBlock == 0 {
			moduleWithUse.InitialBlock = usedModule.InitialBlock
		}

		moduleWithUse.Output = usedModule.Output
		moduleWithUse.Kind = usedModule.Kind
	}
	return nil
}

func checkEqualInputs(moduleWithUse, usedModule *pbsubstreams.Module, manifestModuleWithUse *Module, packageModulesMapping map[string]*pbsubstreams.Module) error {
	for index, input := range moduleWithUse.Inputs {
		usedModuleInput := usedModule.Inputs[index]
		if input.GetParams() != nil {
			if usedModuleInput.GetParams() == nil {
				return fmt.Errorf("module %q: input %q is not a params type", manifestModuleWithUse.Name, input.String())
			}
			if input.GetParams().Value != usedModuleInput.GetParams().Value {
				return fmt.Errorf("module %q: input %q has different value than the used module %q: input %q", manifestModuleWithUse.Name, input.String(), manifestModuleWithUse.Use, usedModuleInput.String())
			}
			continue
		}
		if input.GetSource() != nil {
			if usedModuleInput.GetSource() == nil {
				return fmt.Errorf("module %q: input %q is not a source type", manifestModuleWithUse.Name, input.String())
			}
			if input.GetSource().Type != usedModuleInput.GetSource().Type {
				return fmt.Errorf("module %q: input %q has different source than the used module %q: input %q", manifestModuleWithUse.Name, input.String(), manifestModuleWithUse.Use, usedModuleInput.String())
			}
			continue
		}
		if input.GetStore() != nil {
			if usedModuleInput.GetStore() == nil {
				return fmt.Errorf("module %q: input %q is not a store type", manifestModuleWithUse.Name, input.String())
			}
			if input.GetStore().GetMode() != usedModuleInput.GetStore().GetMode() {
				return fmt.Errorf("module %q: input %q has different mode than the used module %q: input %q", manifestModuleWithUse.Name, input.String(), manifestModuleWithUse.Use, usedModuleInput.String())
			}

			inputStoreModuleName := input.GetStore().ModuleName
			_, found := packageModulesMapping[inputStoreModuleName]
			if !found {
				return fmt.Errorf("module %q: input %q store module %q not found", manifestModuleWithUse.Name, input.String(), inputStoreModuleName)
			}

			usedModuleStoreMapModuleName := usedModuleInput.GetStore().ModuleName
			_, found = packageModulesMapping[usedModuleStoreMapModuleName]
			if !found {
				return fmt.Errorf("module %q: input %q store module %q not found", manifestModuleWithUse.Name, usedModuleInput.String(), usedModuleStoreMapModuleName)
			}

			if packageModulesMapping[inputStoreModuleName].Output.Type != packageModulesMapping[usedModuleStoreMapModuleName].Output.Type {
				return fmt.Errorf("module %q: input %q has different output than the used module %q: input %q", manifestModuleWithUse.Name, input.String(), manifestModuleWithUse.Use, usedModuleInput.String())
			}
			continue

		}
		if input.GetMap() != nil {
			if usedModuleInput.GetMap() == nil {
				return fmt.Errorf("module %q: input %q is not a map type", manifestModuleWithUse.Name, input.String())
			}

			inputMapModuleName := input.GetMap().ModuleName
			_, found := packageModulesMapping[inputMapModuleName]
			if !found {
				return fmt.Errorf("module %q: input %q map module %q not found", manifestModuleWithUse.Name, input.String(), inputMapModuleName)
			}

			usedModuleInputMapModuleName := usedModuleInput.GetMap().ModuleName
			_, found = packageModulesMapping[usedModuleInputMapModuleName]
			if !found {
				return fmt.Errorf("module %q: input %q map module %q not found", manifestModuleWithUse.Name, usedModuleInput.String(), usedModuleInputMapModuleName)
			}

			if packageModulesMapping[inputMapModuleName].Output.Type != packageModulesMapping[usedModuleInputMapModuleName].Output.Type {
				return fmt.Errorf("module %q: input %q has different output than the used module %q: input %q", manifestModuleWithUse.Name, input.String(), manifestModuleWithUse.Use, usedModuleInput.String())
			}
			continue
		}
	}
	return nil
}

func (r *manifestConverter) manifestToPkg(manif *Manifest) (*pbsubstreams.Package, []*desc.FileDescriptor, *dynamic.Message, error) {
	pkg, err := r.convertToPkg(manif)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to convert manifest to pkg: %w", err)
	}

	protoDefinitions, err := loadProtobufs(pkg, manif)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("error loading protobuf: %w", err)
	}

	if manif.Package.Image != "" {
		if err := loadImage(pkg, manif); err != nil {
			return nil, nil, nil, fmt.Errorf("error loading image: %w", err)
		}
	}

	if err := loadImports(pkg, manif); err != nil {
		return nil, nil, nil, fmt.Errorf("error loading imports: %w", err)
	}

	if err := r.loadSinkConfig(pkg, manif); err != nil {
		return nil, nil, nil, fmt.Errorf("error parsing sink configuration: %w", err)
	}

	if err := handleUseModules(pkg, manif); err != nil {
		return nil, nil, nil, fmt.Errorf("error handling use modules: %w", err)
	}

	return pkg, protoDefinitions, r.sinkConfigDynamicMessage, nil
}

func (m *Manifest) readFileFromName(filename string) ([]byte, error) {
	fileNameFound, err := searchExistingCaseInsensitiveFileName(m.Workdir, filename)
	if err != nil {
		return nil, err
	}

	filePath := filepath.Join(m.Workdir, fileNameFound)
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read %s: %w", fileNameFound, err)
	}
	return content, nil
}

func (r *manifestConverter) convertToPkg(m *Manifest) (pkg *pbsubstreams.Package, err error) {
	doc := m.Package.Doc
	if doc == "" {
		readmeContent, err := m.readFileFromName("README.md")
		if err != nil {
			if !os.IsNotExist(err) {
				return nil, fmt.Errorf("reading file: %w", err)
			}
			readmeContent, err = m.readFileFromName("README")
			if err != nil {
				if !os.IsNotExist(err) {
					return nil, fmt.Errorf("reading file: %w", err)
				}
			}
		}

		doc = string(readmeContent)
	}

	pkgMeta := &pbsubstreams.PackageMetadata{
		Version: m.Package.Version,
		Url:     m.Package.URL,
		Name:    m.Package.Name,
		Doc:     doc,
	}
	pkg = &pbsubstreams.Package{
		Version:     1,
		PackageMeta: []*pbsubstreams.PackageMetadata{pkgMeta},
		Modules:     &pbsubstreams.Modules{},
		Network:     m.Network,
	}

	if m.Networks != nil {
		pkg.Networks = make(map[string]*pbsubstreams.NetworkParams)
		for k, v := range m.Networks {
			params := &pbsubstreams.NetworkParams{}

			if v.InitialBlocks != nil {
				params.InitialBlocks = make(map[string]uint64)
			}
			for kk, vv := range v.InitialBlocks {
				params.InitialBlocks[kk] = vv
			}

			if v.Params != nil {
				params.Params = make(map[string]string)
			}
			for kk, vv := range v.Params {
				params.Params[kk] = vv
			}

			pkg.Networks[k] = params
		}
	}
	moduleCodeIndexes := map[string]int{}

	for _, mod := range m.Modules {
		pbmeta := &pbsubstreams.ModuleMetadata{
			Doc: mod.Doc,
		}
		var pbmod *pbsubstreams.Module

		binaryName := "default"
		implicit := ""
		if mod.Binary != "" {
			binaryName = mod.Binary
			implicit = "(implicit) "
		}
		binaryDef, found := m.Binaries[binaryName]
		if !found {
			return nil, fmt.Errorf("module %q refers to %sbinary %q, which is not defined in the 'binaries' section of the manifest", mod.Name, implicit, binaryName)
		}

		switch binaryDef.Type {
		case "wasm/rust-v1":
			// OPTIM(abourget): also check if it's not already in
			// `Binaries`, by comparing its, length + hash or value.
			codeIndex, found := moduleCodeIndexes[binaryDef.File]
			if !found {
				codePath := m.resolvePath(binaryDef.File)
				var byteCode []byte
				if !r.skipSourceCodeImportValidation {
					byteCode, err = os.ReadFile(codePath)
					if err != nil {
						return nil, fmt.Errorf("failed to read source code %q: %w", codePath, err)
					}
				}
				pkg.Modules.Binaries = append(pkg.Modules.Binaries, &pbsubstreams.Binary{Type: binaryDef.Type, Content: byteCode})
				codeIndex = len(pkg.Modules.Binaries) - 1
				moduleCodeIndexes[binaryDef.File] = codeIndex
			}
			pbmod, err = mod.ToProtoWASM(uint32(codeIndex))
		default:
			return nil, fmt.Errorf("module %q: invalid code type %q", mod.Name, binaryDef.Type)
		}
		if err != nil {
			return nil, err
		}

		pkg.ModuleMeta = append(pkg.ModuleMeta, pbmeta)
		pkg.Modules.Modules = append(pkg.Modules.Modules, pbmod)
	}

	for modName, paramValue := range m.Params {
		var modFound bool
		for _, mod := range pkg.Modules.Modules {
			if mod.Name == modName {
				if len(mod.Inputs) == 0 {
					return nil, fmt.Errorf("params value defined for module %q but module has no inputs defined, add 'params: string' to 'inputs' for module", modName)
				}
				p := mod.Inputs[0].GetParams()
				if p == nil {
					return nil, fmt.Errorf("params value defined for module %q: module %q does not have 'params' as its first input type", modName, modName)
				}
				p.Value = paramValue
				modFound = true
			}
		}
		if !modFound {
			return nil, fmt.Errorf("params value defined for module %q, but such module is not defined", modName)
		}
	}

	return
}
