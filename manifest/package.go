package manifest

import (
	"fmt"
	"io/ioutil"
	"strings"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"google.golang.org/protobuf/proto"
)

func New(inputFile string) (m *pbsubstreams.Package, err error) {
	if strings.HasSuffix(inputFile, ".yaml") {
		return NewFromYAML(inputFile)
	}
	return NewFromPackageFile(inputFile)
}

func NewFromPackageFile(inputFile string) (pkg *pbsubstreams.Package, err error) {
	pkg = &pbsubstreams.Package{}
	cnt, err := ioutil.ReadFile(inputFile)
	if err != nil {
		return nil, err
	}
	if err := proto.Unmarshal(cnt, pkg); err != nil {
		return nil, err
	}
	return pkg, nil
}

func NewFromYAML(inputPath string) (pkg *pbsubstreams.Package, err error) {
	manif, err := loadManifestFile(inputPath)
	if err != nil {
		return nil, err
	}

	if err := manif.loadSourceCode(); err != nil {
		return nil, err
	}

	pkg, err = manif.intoPackage()
	if err != nil {
		return nil, err
	}

	if err := loadImports(pkg, manif); err != nil {
		return nil, err
	}

	if err := validateClientPackage(pkg); err != nil {
		return nil, err
	}

	if err := loadProtobufs(pkg, manif); err != nil {
	}

	if err := ValidateModules(pkg.Modules); err != nil {
		return nil, err
	}

	return pkg, nil
}

func (m *Manifest) intoPackage() (pkg *pbsubstreams.Package, err error) {
	pkgMeta := &pbsubstreams.PackageMetadata{
		Version: m.Package.Version,
		Url:     m.Package.URL,
		Name:    m.Package.Name,
	}
	pkg = &pbsubstreams.Package{
		PackageMeta: []*pbsubstreams.PackageMetadata{pkgMeta},
		Modules:     &pbsubstreams.Modules{},
	}

	moduleCodeIndexes := map[string]int{}
	for _, mod := range m.Modules {
		pbmeta := &pbsubstreams.ModuleMetadata{
			Doc: mod.Doc,
		}
		var pbmod *pbsubstreams.Module

		switch mod.Code.Type {
		case "native":
			pbmod, err = mod.ToProtoNative()
		case "wasm/rust-v1":
			codeIndex, found := moduleCodeIndexes[mod.Code.File]
			if !found {
				codeIndex, err = m.loadCode(mod.Code.File, pkg.Modules)
				if err != nil {
					return nil, fmt.Errorf("loading code: %w", err)
				}
				moduleCodeIndexes[mod.Code.File] = codeIndex
			}

			pbmod, err = mod.ToProtoWASM(uint32(codeIndex))
		default:
			return nil, fmt.Errorf("module %q: invalid code type %q", mod.Name, mod.Code.Type)
		}
		if err != nil {
			return nil, err
		}

		pkg.ModuleMeta = append(pkg.ModuleMeta, pbmeta)
		pkg.Modules.Modules = append(pkg.Modules.Modules, pbmod)
	}

	return
}

var validValueTypes = map[string]bool{
	"bigint":   true,
	"int64":    true,
	"bigfloat": true,
	"bytes":    true,
	"string":   true,
	"proto":    true,
}

// ValidateProtoReferences is run only by the client, as the server
// doesn't have access to the full Package, but only the
// modules. Also, the proto references are used solely by the client.
//
// WARN: put ANY MODULES validation that need to be applied by the
// server in `ValidateModules`.
func validateClientPackage(pkg *pbsubstreams.Package) error {
	for _, spkg := range pkg.PackageMeta {
		_ = spkg
		// TODO: Validate syntax of spkg.Name
	}

	for _, mod := range pkg.Modules.Modules {
		switch i := mod.Kind.(type) {
		case *pbsubstreams.Module_KindStore_:
			valueType := i.KindStore.ValueType
			if strings.HasPrefix(valueType, "proto:") {

			} else if !validValueTypes[valueType] {
				return fmt.Errorf("module %q: invalid valueType %q", mod.Name, valueType)
			}
		}

		for _, in := range mod.Inputs {
			_ = in
			// TODO: do more proto ref checking for those inputs listed
		}
	}

	// TODO: Loop through inputs, outputs, and check that all internal proto references are satisfied by the FileDescriptors
	return nil
}

// ValidateModules is run both by the client _and_ the server.
func ValidateModules(mods *pbsubstreams.Modules) error {
	var sumCode int
	for _, modCode := range mods.ModulesCode {
		sumCode += len(modCode)
	}
	if sumCode > 100_000_000 {
		return fmt.Errorf("limit of 100MB of module code size reached")
	}

	for _, mod := range mods.Modules {
		if !ModuleNameRegexp.MatchString(mod.Name) {
			return fmt.Errorf("module name %s does not match regex %s", mod.Name, ModuleNameRegexp.String())
		}

		for idx, in := range mod.Inputs {
			switch i := in.Input.(type) {
			case *pbsubstreams.Module_Input_Map_:
				// TODO: validate that i.ModuleName exists in the modules list
			case *pbsubstreams.Module_Input_Store_:
				// TODO: validate that i.ModuleName exists in the modules list
				switch i.Store.Mode {
				case pbsubstreams.Module_Input_Store_GET, pbsubstreams.Module_Input_Store_DELTAS:
				default:
					return fmt.Errorf("module %q: input index %d: unknown store mode value %d", mod.Name, idx, i.Store.Mode)
				}
			}
		}
	}
	return nil
}

func loadImports(pkg *pbsubstreams.Package, manif *Manifest) error {
	for _, kv := range manif.Imports {
		importName := kv[0]
		importPath := kv[1]

		_ = importName
		_ = importPath
	}
	// loop through the Manifest, and get the `imports` statements,
	// pull the Package files from Disk, and merge them into this one
	return nil
}
