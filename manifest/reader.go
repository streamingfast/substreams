package manifest

import (
	"fmt"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"go.uber.org/zap"
	"golang.org/x/mod/semver"
	"google.golang.org/protobuf/proto"
	"io/ioutil"
	"net/http"
	"net/url"
	"path"
	"path/filepath"
	"strings"
)

type Options func(r *Reader) *Reader

func SkipSourceCodeReader() Options {
	return func(r *Reader) *Reader {
		r.skipSourceCodeImportValidation = true
		return r
	}
}

type Reader struct {
	input string

	//options
	skipSourceCodeImportValidation bool
}

func NewReader(input string, opts ...Options) *Reader {
	r := &Reader{input: input}
	for _, opt := range opts {
		r = opt(r)
	}
	return r
}

func (r *Reader) Read() (*pbsubstreams.Package, error) {
	if u, err := url.Parse(r.input); err == nil && u.Scheme == "http" || u.Scheme == "https" {
		return r.newPkgFromURL(r.input)
	}

	if strings.HasSuffix(r.input, ".yaml") {
		return r.newPkgFromManifest(r.input)
	}

	return r.newPkgFromFile(r.input)
}

func (r *Reader) newPkgFromFile(inputFilePath string) (pkg *pbsubstreams.Package, err error) {
	cnt, err := ioutil.ReadFile(inputFilePath)
	if err != nil {
		return nil, fmt.Errorf("error reading %q: %w", inputFilePath, err)
	}

	return r.fromContents(cnt)
}

func (r *Reader) newPkgFromURL(fileURL string) (pkg *pbsubstreams.Package, err error) {
	resp, err := http.DefaultClient.Get(fileURL)
	if err != nil {
		return nil, fmt.Errorf("error downloading %q: %w", fileURL, err)
	}
	cnt, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading %q: %w", fileURL, err)
	}
	return r.fromContents(cnt)
}

func (r *Reader) newPkgFromManifest(inputPath string) (pkg *pbsubstreams.Package, err error) {
	manif, err := loadManifestFile(inputPath)
	if err != nil {
		return nil, err
	}

	pkg, err = r.manifestToPkg(manif, r.skipSourceCodeImportValidation)
	if err != nil {
		return nil, err
	}

	if err := r.validate(pkg); err != nil {
		return nil, fmt.Errorf("failed validation: %w", err)
	}

	return pkg, nil
}

func (r *Reader) fromContents(contents []byte) (pkg *pbsubstreams.Package, err error) {
	pkg = &pbsubstreams.Package{}
	if err := proto.Unmarshal(contents, pkg); err != nil {
		return nil, fmt.Errorf("failed to unmarshall content: %w", err)
	}

	if err := r.validate(pkg); err != nil {
		return nil, fmt.Errorf("failed validation: %w", err)
	}

	return pkg, nil
}

func (r *Reader) validate(pkg *pbsubstreams.Package) error {
	if err := r.validatePackage(pkg); err != nil {
		return fmt.Errorf("failed packagae validation: %w", err)
	}

	if err := r.validateModules(pkg.Modules); err != nil {
		return fmt.Errorf("failed module validation: %w", err)
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
func (r *Reader) validatePackage(pkg *pbsubstreams.Package) error {
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

// ValidateModules is run both by the client _and_ the server.
func (r *Reader) validateModules(mods *pbsubstreams.Modules) error {
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

func loadManifestFile(inputPath string) (*Manifest, error) {
	m, err := decodeYamlManifestFromFile(inputPath)
	if err != nil {
		return nil, fmt.Errorf("decoding yaml: %w", err)
	}

	absoluteManifestPath, err := filepath.Abs(inputPath)
	if err != nil {
		return nil, fmt.Errorf("getting absolute path: %w", err)
	}

	m.Workdir = path.Dir(absoluteManifestPath)

	if m.SpecVersion != "v0.1.0" {
		return nil, fmt.Errorf("invalid 'specVersion', must be v0.1.0")
	}

	// TODO: put some limits on the NUMBER of modules (max 50 ?)
	// TODO: put a limit on the SIZE of the WASM payload (max 10MB per binary?)

	for _, s := range m.Modules {
		// TODO: let's make sure this is also checked when received in Protobuf in a remote request.

		switch s.Kind {
		case ModuleKindMap:
			if s.Output.Type == "" {
				return nil, fmt.Errorf("stream %q: missing 'output.type' for kind 'map'", s.Name)
			}
			// TODO: check protobuf
			if s.Code.Entrypoint == "" {
				s.Code.Entrypoint = "map"
			}
		case ModuleKindStore:
			if err := validateStoreBuilder(s); err != nil {
				return nil, fmt.Errorf("stream %q: %w", s.Name, err)
			}

			if s.Code.Entrypoint == "" {
				// TODO: let's make sure this is validated also when analyzing some incoming protobuf version
				// of this.
				s.Code.Entrypoint = "build_state"
			}

		default:
			return nil, fmt.Errorf("stream %q: invalid kind %q", s.Name, s.Kind)
		}

		for _, input := range s.Inputs {
			if err := input.parse(); err != nil {
				return nil, fmt.Errorf("module %q: %w", s.Name, err)
			}
		}
	}

	return m, nil
}

func loadImports(pkg *pbsubstreams.Package, manif *Manifest) error {
	for _, kv := range manif.Imports {
		importName := kv[0]
		importPath := kv[1]

		subpkgReader := NewReader(importPath)
		subpkg, err := subpkgReader.Read()
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
		seenFiles[*file.Name] = true
	}

	for _, file := range src.ProtoFiles {
		key := *file.Name
		if seenFiles[key] {
			zlog.Debug("skipping protofile already seen", zap.String("proto_file", *file.Name))
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

// manifestToPkg will take a Manifest object, most likely generated from a YAML file, and will create a Proto Pakcage object
// in some cases we do not want to validate the package and ensure that all the code and dependencies are there fro example
// when we are using the generated package transitively
func (r *Reader) manifestToPkg(m *Manifest, ignoreError bool) (*pbsubstreams.Package, error) {
	pkg, err := r.convertToPkg(m)
	if err != nil && !ignoreError {
		return nil, fmt.Errorf("failed to convert manifest to pkg: %w", err)
	}

	if err := loadProtobufs(pkg, m); err != nil {
		return nil, fmt.Errorf("error loading protobuf: %w", err)
	}

	if err := loadImports(pkg, m); err != nil {
		return nil, fmt.Errorf("error loading imports: %w", err)
	}

	return pkg, nil
}

func (r *Reader) convertToPkg(m *Manifest) (pkg *pbsubstreams.Package, err error) {
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
			// OPTIM(abourget): also check if it's not already in
			// `ModulesCode`, by comparing its, length + hash or value.
			codeIndex, found := moduleCodeIndexes[mod.Code.File]
			if !found {
				codePath := mod.Code.File
				byteCode, err := ioutil.ReadFile(codePath)
				if err != nil {
					return nil, fmt.Errorf("failed to read source code %q: %w", codePath, err)
				}
				pkg.Modules.ModulesCode = append(pkg.Modules.ModulesCode, byteCode)
				codeIndex = len(pkg.Modules.ModulesCode) - 1
				moduleCodeIndexes[mod.Code.File] = codeIndex
			}
			pkg.ModuleMeta = append(pkg.ModuleMeta, pbmeta)
			pbmod, err = mod.ToProtoWASM(uint32(codeIndex))
		default:
			return nil, fmt.Errorf("module %q: invalid code type %q", mod.Name, mod.Code.Type)
		}
		if err != nil {
			return nil, err
		}
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
