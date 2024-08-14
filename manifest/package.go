package manifest

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"

	"github.com/jhump/protoreflect/desc"
	"github.com/jhump/protoreflect/dynamic"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/sqe"
	"google.golang.org/protobuf/proto"
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
		if s.BlockFilter != nil && !s.BlockFilter.IsEmpty() {
			ctx := context.Background()
			if err := validateQuery(ctx, s.BlockFilter.Query, manif.Params[s.Name]); err != nil {
				return fmt.Errorf("stream %q: %w", s.Name, err)
			}
		}
		// TODO: let's make sure this is also checked when received in Protobuf in a remote request.
		switch s.Kind {
		case ModuleKindMap:
			if s.Output.Type == "" {
				return fmt.Errorf("stream %q: missing 'output.type' for kind 'map'", s.Name)
			}
			if s.Use != "" {
				return fmt.Errorf("stream %q: 'use' is not allowed for kind 'map'", s.Name)
			}
		case ModuleKindStore:
			if err := validateStoreBuilder(s); err != nil {
				return fmt.Errorf("stream %q: %w", s.Name, err)
			}
			if s.Use != "" {
				return fmt.Errorf("stream %q: 'use' is not allowed for kind 'store'", s.Name)
			}
		case ModuleKindBlockIndex:
			if s.Inputs == nil {
				return fmt.Errorf("stream %q: block index module should have inputs", s.Name)
			}

			for _, input := range s.Inputs {
				if input.IsParams() {
					return fmt.Errorf("stream %q: block index module cannot have params input", s.Name)
				}
			}

			if s.BlockFilter != nil {
				return fmt.Errorf("stream %q: block index module cannot have block filter", s.Name)
			}

			if s.Output.Type != "proto:sf.substreams.index.v1.Keys" {
				return fmt.Errorf("stream %q: block index module must have output type 'proto:sf.substreams.index.v1.Keys'", s.Name)
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

func validateQuery(ctx context.Context, query BlockFilterQuery, param string) error {
	var q string
	switch {
	case query.String != "" && query.Params:
		return fmt.Errorf("only one of 'string' or 'params' can be set")
	case query.String != "":
		q = query.String
	case query.Params:
		q = param
	default:
		return fmt.Errorf("missing query")
	}

	_, err := sqe.Parse(ctx, q)
	if err != nil {
		return fmt.Errorf("invalid query: %w", err)
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

		if err := checkUseInputs(moduleWithUse, usedModule, manifestModule, packageModulesMapping); err != nil {
			return fmt.Errorf("checking inputs for module %q: %w", manifestModule.Name, err)
		}

		if moduleWithUse.BlockFilter == nil {
			moduleWithUse.BlockFilter = usedModule.BlockFilter
		}

		moduleWithUse.BinaryIndex = usedModule.BinaryIndex
		moduleWithUse.BinaryEntrypoint = usedModule.BinaryEntrypoint

		moduleWithUse.Output = usedModule.Output
		moduleWithUse.Kind = usedModule.Kind
	}
	return nil
}

func isEmptyMessage(msg *pbsubstreams.Module_BlockFilter) bool {
	emptyMessage := &pbsubstreams.Module_BlockFilter{}
	return proto.Equal(msg, emptyMessage)
}

func checkUseInputs(moduleWithUse, usedModule *pbsubstreams.Module, manifestModuleWithUse *Module, packageModulesMapping map[string]*pbsubstreams.Module) error {
	if moduleWithUse.Inputs == nil {
		moduleWithUse.Inputs = usedModule.Inputs
	}

	if len(moduleWithUse.Inputs) != len(usedModule.Inputs) {
		return fmt.Errorf("module %q inputs count mismatch with the used module %q", manifestModuleWithUse.Name, manifestModuleWithUse.Use)
	}

	for index, input := range moduleWithUse.Inputs {
		usedModuleInput := usedModule.Inputs[index]

		switch {
		case input.GetSource() != nil:
			if usedModuleInput.GetSource() == nil {
				return fmt.Errorf("module %q: input %q is not a source type", manifestModuleWithUse.Name, input.String())
			}
			if input.GetSource().Type != usedModuleInput.GetSource().Type {
				return fmt.Errorf("module %q: input %q has different source than the used module %q: input %q", manifestModuleWithUse.Name, input.String(), manifestModuleWithUse.Use, usedModuleInput.String())
			}

		case input.GetParams() != nil:
			if usedModuleInput.GetParams() == nil {
				return fmt.Errorf("module %q: input %q is not a params type", manifestModuleWithUse.Name, input.String())
			}

		case input.GetStore() != nil:
			if usedModuleInput.GetStore() == nil {
				return fmt.Errorf("module %q: input %q is not a store type", manifestModuleWithUse.Name, input.String())
			}
			if input.GetStore().GetMode() != usedModuleInput.GetStore().GetMode() {
				return fmt.Errorf("module %q: input %q has different mode than the used module %q: input %q", manifestModuleWithUse.Name, input.String(), manifestModuleWithUse.Use, usedModuleInput.String())
			}

		case input.GetMap() != nil:
			if usedModuleInput.GetMap() == nil {
				return fmt.Errorf("module %q: input %q is not a map type", manifestModuleWithUse.Name, input.String())
			}

			curMod, found := packageModulesMapping[input.GetMap().ModuleName]
			if !found {
				return fmt.Errorf("module %q: input %q map module %q not found", manifestModuleWithUse.Name, input.String(), input.GetMap().ModuleName)
			}

			usedMod, found := packageModulesMapping[usedModuleInput.GetMap().ModuleName]
			if !found {
				return fmt.Errorf("module %q: input %q map module %q not found", manifestModuleWithUse.Name, usedModuleInput.String(), usedModuleInput.GetMap().ModuleName)
			}

			if curMod.Output.Type != usedMod.Output.Type {
				return fmt.Errorf("module %q: input %q has different output than the used module %q: input %q", manifestModuleWithUse.Name, input.String(), manifestModuleWithUse.Use, usedModuleInput.String())
			}
		}
	}

	return nil
}

func (r *manifestConverter) manifestToPkg(manif *Manifest) (*pbsubstreams.Package, []*desc.FileDescriptor, *dynamic.Message, error) {
	pkg, err := r.convertToPkg(manif)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to convert manifest to pkg: %w", err)
	}

	if err := loadImports(pkg, manif); err != nil {
		return nil, nil, nil, fmt.Errorf("error loading imports: %w", err)
	}

	var protoFiles []*desc.FileDescriptor

	fromBufBuild, err := loadDescriptorSets(pkg, manif)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("loading protobuf: %w", err)
	}
	protoFiles = append(protoFiles, fromBufBuild...)

	fromLocalFiles, err := loadLocalProtobufs(pkg, manif)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("loading protobuf: %w", err)
	}

	protoFiles = append(protoFiles, fromLocalFiles...)

	if manif.Package.Image != "" {
		if err := loadImage(pkg, manif); err != nil {
			return nil, nil, nil, fmt.Errorf("loading image: %w", err)
		}
	}

	if err := r.loadSinkConfig(pkg, manif); err != nil {
		return nil, nil, nil, fmt.Errorf("parsing sink configuration: %w", err)
	}

	if err := handleUseModules(pkg, manif); err != nil {
		return nil, nil, nil, fmt.Errorf("handling use modules: %w", err)
	}

	if err := handleParams(pkg, manif); err != nil {
		return nil, nil, nil, fmt.Errorf("handling params: %w", err)
	}

	//Set all empty blockFilter to nil (enables to  override blockFilter by nil for used modules)
	handleEmptyBlockFilter(pkg, manif)

	return pkg, protoFiles, r.sinkConfigDynamicMessage, nil
}

func handleEmptyBlockFilter(pkg *pbsubstreams.Package, manif *Manifest) {
	for _, mod := range pkg.Modules.Modules {
		if isEmptyMessage(mod.BlockFilter) {
			mod.BlockFilter = nil
		}
	}
}

func handleParams(pkg *pbsubstreams.Package, manif *Manifest) error {
	for modName, paramValue := range manif.Params {
		var modFound bool
		for _, mod := range pkg.Modules.Modules {
			if mod.Name == modName {
				if len(mod.Inputs) == 0 {
					return fmt.Errorf("params value defined for module %q but module has no inputs defined, add 'params: string' to 'inputs' for module", modName)
				}

				p := mod.Inputs[0].GetParams()
				if p == nil {
					return fmt.Errorf("params value defined for module %q: module %q does not have 'params' as its first input type", modName, modName)
				}
				p.Value = paramValue
				modFound = true
			}
		}
		if !modFound {
			return fmt.Errorf("params value defined for module %q, but such module is not defined", modName)
		}
	}
	return nil
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
	if doc != "" {
		fmt.Println("Deprecated: the 'package.doc' field is deprecated. The README.md file is picked up instead.")
	}
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
			fmt.Println("Warning: README.md file not found, no documentation will be packaged")
			err = nil
		}
	}

	doc = string(readmeContent)

	pkgMeta := &pbsubstreams.PackageMetadata{
		Version:     m.Package.Version,
		Url:         m.Package.URL,
		Name:        m.Package.Name,
		Description: m.Package.Description,
		Doc:         doc,
	}

	pkg = &pbsubstreams.Package{
		Version:      1,
		PackageMeta:  []*pbsubstreams.PackageMetadata{pkgMeta},
		Modules:      &pbsubstreams.Modules{},
		Network:      m.Network,
		BlockFilters: m.BlockFilters,
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
		pkg.ModuleMeta = append(pkg.ModuleMeta, &pbsubstreams.ModuleMetadata{
			Doc: mod.Doc,
		})

		if mod.Use != "" {
			pbmod, err := mod.ToProtoWASM(0) // the binary index and module will be overriden by th 'use'
			if err != nil {
				return nil, fmt.Errorf("handling used module %q for module %q: %w", mod.Use, mod.Name, err)
			}

			pkg.Modules.Modules = append(pkg.Modules.Modules, pbmod)

			continue
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

		wasmCodeTypeID, _ := SplitBinaryType(binaryDef.Type)

		switch wasmCodeTypeID {
		case "wasm/rust-v1", "wasip1/tinygo-v1":
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
			if err != nil {
				return nil, err
			}

		default:
			return nil, fmt.Errorf("module %q: invalid code type %q", mod.Name, binaryDef.Type)
		}

		pkg.Modules.Modules = append(pkg.Modules.Modules, pbmod)
	}

	return
}
