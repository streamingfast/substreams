package manifest

import (
	"crypto/sha256"
	"fmt"
	"io/ioutil"
	"strings"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"golang.org/x/mod/semver"
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

	if err := validatePackage(pkg); err != nil {
		return nil, fmt.Errorf("package validation error: %q: %w", inputFile, err)
	}

	if err := ValidateModules(pkg.Modules); err != nil {
		return nil, fmt.Errorf("validation error for %q: %w", inputFile, err)
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

	if err := loadProtobufs(pkg, manif); err != nil {
		return nil, err
	}

	if err := loadImports(pkg, manif); err != nil {
		return nil, err
	}

	if err := validatePackage(pkg); err != nil {
		return nil, err
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
		Version:     1,
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

// ValidateModules is run both by the client _and_ the server.
func ValidateModules(mods *pbsubstreams.Modules) error {
	var sumCode int
	for _, modCode := range mods.ModulesCode {
		sumCode += len(modCode)
	}
	if sumCode > 100_000_000 {
		return fmt.Errorf("limit of 100MB of module code size reached")
	}
	if len(mods.Modules) > 100 {
		return fmt.Errorf("limit of 100 modules reached")
	}

	for _, mod := range mods.Modules {
		for _, segment := range strings.Split(mod.Name, ":") {
			if !moduleNameRegexp.MatchString(segment) {
				return fmt.Errorf("module %q: segment %q does not match regex %s", mod.Name, segment, moduleNameRegexp.String())
			}
		}

		if len(mod.Inputs) > 30 {
			return fmt.Errorf("limit of 30 inputs for a given module (%q) reached", mod.Name)
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

// validatePackage validates a package just produced or just read from
// disk.
//
// validatePackage is run only by the client, as the server doesn't
// have access to the full Package.
//
// WARN: put ANY MODULES validation that need to be applied by the
// server in `ValidateModules`.
func validatePackage(pkg *pbsubstreams.Package) error {
	if len(pkg.ModuleMeta) != len(pkg.Modules.Modules) {
		return fmt.Errorf("inconsistent package, metadata for modules not same length as modules list")
	}
	if pkg.Version < 1 {
		return fmt.Errorf("unrecognized package version: %d (are you sure this is a substreams package?)", pkg.Version)
	}
	if len(pkg.PackageMeta) == 0 {
		return fmt.Errorf("no package metadata present in package (are you sure this is a substreams package?)")
	}

	for _, spkg := range pkg.PackageMeta {
		if !moduleNameRegexp.MatchString(spkg.Name) {
			return fmt.Errorf("package %q: invalid name: must match %s", spkg.Name, moduleNameRegexp.String())
		}
		if !semver.IsValid(spkg.Version) {
			return fmt.Errorf("package %q: version %q should match Semver", spkg.Name, spkg.Version)
		}
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

		switch modCode := mod.Code.(type) {
		case *pbsubstreams.Module_WasmCode_:
			if int(modCode.WasmCode.Index) >= len(pkg.Modules.ModulesCode) {
				return fmt.Errorf("invalid internal reference to modules code index for module %q", mod.Name)
			}
		case *pbsubstreams.Module_NativeCode_:
		default:
			return fmt.Errorf("unsupported code type %s for package %q", mod.Code, pkg.PackageMeta[0].Name)
		}

		for _, in := range mod.Inputs {
			_ = in
			// TODO: do more proto ref checking for those inputs listed
		}
	}

	// TODO: Loop through inputs, outputs, and check that all internal proto references are satisfied by the FileDescriptors

	return nil
}

func loadImports(pkg *pbsubstreams.Package, manif *Manifest) error {
	for _, kv := range manif.Imports {
		importName := kv[0]
		importPath := kv[1]

		subpkg, err := New(importPath)
		if err != nil {
			return fmt.Errorf("importing %q: %w", importPath, err)
		}

		prefixModules(subpkg.Modules.Modules, importName)
		reindexAndMergePackage(subpkg, pkg)
		mergeProtoFiles(subpkg, pkg)
	}
	// loop through the Manifest, and get the `imports` statements,
	// pull the Package files from Disk, and merge them into this one
	return nil
}

const PrefixSeparator = ":"

func prefixModules(mods []*pbsubstreams.Module, prefix string) {
	for _, mod := range mods {
		mod.Name = prefix + PrefixSeparator + mod.Name
		for _, inputIface := range mod.Inputs {
			switch input := inputIface.Input.(type) {
			case *pbsubstreams.Module_Input_Source_:
			case *pbsubstreams.Module_Input_Store_:
				input.Store.ModuleName = prefix + PrefixSeparator + input.Store.ModuleName
			case *pbsubstreams.Module_Input_Map_:
				input.Map.ModuleName = prefix + PrefixSeparator + input.Map.ModuleName
			default:
				panic(fmt.Sprintf("unsupported module type %s", inputIface.Input))
			}
		}
	}
}

// mergeAndReindexPackages consumes the `src` Package into `dest`, and
// modifies `src`.
func reindexAndMergePackage(src, dest *pbsubstreams.Package) {
	newBasePackageIndex := len(dest.PackageMeta)
	newBaseModuleCodeIndex := len(dest.Modules.ModulesCode)

	for _, modMeta := range src.ModuleMeta {
		modMeta.PackageIndex += uint64(newBasePackageIndex)
	}
	for _, mod := range src.Modules.Modules {
		if modCode, ok := mod.Code.(*pbsubstreams.Module_WasmCode_); ok {
			modCode.WasmCode.Index += uint32(newBaseModuleCodeIndex)
		}
	}
	dest.Modules.Modules = append(dest.Modules.Modules, src.Modules.Modules...)
	dest.Modules.ModulesCode = append(dest.Modules.ModulesCode, src.Modules.ModulesCode...)
	dest.ModuleMeta = append(dest.ModuleMeta, src.ModuleMeta...)
	dest.PackageMeta = append(dest.PackageMeta, src.PackageMeta...)
}

func mergeProtoFiles(src, dest *pbsubstreams.Package) {
	seenFiles := map[string]bool{}

	for _, file := range dest.ProtoFiles {
		cnt, _ := proto.Marshal(file)
		key := fmt.Sprintf("%x", sha256.Sum256(cnt))
		//fmt.Println("in DEST Seen", key, *file.Name)
		seenFiles[key] = true
	}

	for _, file := range src.ProtoFiles {
		cnt, _ := proto.Marshal(file)
		key := fmt.Sprintf("%x", sha256.Sum256(cnt))
		//fmt.Println("Seen in SRC", key, *file.Name)
		if seenFiles[key] {
			continue
		}
		seenFiles[key] = true
		dest.ProtoFiles = append(dest.ProtoFiles, file)
	}

	// TODO: do DEDUPLICATION of those protofiles and/or of the messages therein,
	// otherwise they can duplicate quickly.

	// TODO: eventually, we want the last Message type to win, or perhaps we'd search in reverse order
	// upon `print` or generation? The thing is we'll want tools like `protoc` and `buf` to use the most
	// recent, but it'll simply go in list order..
}
