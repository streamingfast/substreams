package manifest

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/jhump/protoreflect/desc"

	"github.com/streamingfast/dstore"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"go.uber.org/zap"
	"golang.org/x/mod/semver"
	"google.golang.org/protobuf/proto"
)

var httpClient = &http.Client{
	Timeout: 30 * time.Second,
}

type Options func(r *Reader) *Reader

func SkipSourceCodeReader() Options {
	return func(r *Reader) *Reader {
		r.skipSourceCodeImportValidation = true
		return r
	}
}

func SkipModuleOutputTypeValidationReader() Options {
	return func(r *Reader) *Reader {
		r.skipModuleOutputTypeValidation = true
		return r
	}
}

func WithCollectProtoDefinitions(f func(protoDefinitions []*desc.FileDescriptor)) Options {
	return func(r *Reader) *Reader {
		r.collectProtoDefinitionsFunc = f
		return r
	}
}

type Reader struct {
	input                       string
	collectProtoDefinitionsFunc func(protoDefinitions []*desc.FileDescriptor)

	//options
	skipSourceCodeImportValidation bool
	skipModuleOutputTypeValidation bool
}

func NewReader(input string, opts ...Options) *Reader {
	r := &Reader{input: input}
	for _, opt := range opts {
		r = opt(r)
	}

	return r
}

func (r *Reader) Read() (*pbsubstreams.Package, error) {
	if r.IsRemotePackage() {
		return r.newPkgFromURL(r.input)
	}

	if strings.HasSuffix(r.input, ".yaml") {
		pkg, protoDefinitions, err := r.newPkgFromManifest(r.input)
		if err != nil {
			return nil, err
		}
		if r.collectProtoDefinitionsFunc != nil {
			r.collectProtoDefinitionsFunc(protoDefinitions)
		}
		return pkg, nil
	}

	return r.newPkgFromFile(r.input)
}

// IsRemotePackage determines if reader's input to read the manifest is a remote file accessible over
// HTTP/HTTPS, Google Cloud Storage, S3 or Azure Storage.
func (r *Reader) IsRemotePackage() bool {
	u, err := url.Parse(r.input)
	if err != nil {
		return false
	}

	return u.Scheme == "http" || u.Scheme == "https" || u.Scheme == "gs" || u.Scheme == "s3" || u.Scheme == "az"
}

// IsLocalManifest determines if reader's input to read the manifest is a local manifest file, which is determined
// by ensure it's not a remote package and if the file end with `.yaml`.
func (r *Reader) IsLocalManifest() bool {
	if r.IsRemotePackage() {
		return false
	}

	return strings.HasSuffix(r.input, ".yaml")
}

func (r *Reader) newPkgFromFile(inputFilePath string) (pkg *pbsubstreams.Package, err error) {
	cnt, err := os.ReadFile(inputFilePath)
	if err != nil {
		return nil, fmt.Errorf("error reading %q: %w", inputFilePath, err)
	}

	return r.fromContents(cnt)
}

func (r *Reader) newPkgFromURL(fileURL string) (pkg *pbsubstreams.Package, err error) {
	u, err := url.Parse(fileURL)
	if err != nil {
		panic(fmt.Errorf("fileURL %q should have been valid by that execution point but it seems it was not: %w", fileURL, err))
	}

	if u.Scheme == "gs" || u.Scheme == "s3" || u.Scheme == "az" {
		return r.newPkgFromStore(fileURL)
	}

	resp, err := httpClient.Get(fileURL)
	if err != nil {
		return nil, fmt.Errorf("error downloading %q: %w", fileURL, err)
	}

	cnt, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading %q: %w", fileURL, err)
	}

	return r.fromContents(cnt)
}

func (r *Reader) newPkgFromStore(fileURL string) (pkg *pbsubstreams.Package, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cnt, err := dstore.ReadObject(ctx, fileURL)
	if err != nil {
		return nil, fmt.Errorf("error reading %q: %w", fileURL, err)
	}

	return r.fromContents(cnt)
}

func (r *Reader) newPkgFromManifest(inputPath string) (pkg *pbsubstreams.Package, protoDefinitions []*desc.FileDescriptor, err error) {
	manif, err := LoadManifestFile(inputPath)
	if err != nil {
		return nil, nil, err
	}

	pkg, protoDefinitions, err = r.manifestToPkg(manif)
	if err != nil {
		return nil, nil, err
	}

	if err := r.validate(pkg); err != nil {
		return nil, nil, fmt.Errorf("failed validation: %w", err)
	}

	return pkg, protoDefinitions, nil
}

func (r *Reader) fromContents(contents []byte) (pkg *pbsubstreams.Package, err error) {
	pkg = &pbsubstreams.Package{}
	if err := proto.Unmarshal(contents, pkg); err != nil {
		return nil, fmt.Errorf("unmarshalling: %w", err)
	}

	if err := r.validate(pkg); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	return pkg, nil
}

func (r *Reader) validate(pkg *pbsubstreams.Package) error {
	if err := r.validatePackage(pkg); err != nil {
		return fmt.Errorf("package validation failed: %w", err)
	}

	if err := ValidateModules(pkg.Modules); err != nil {
		return fmt.Errorf("module validation failed: %w", err)
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
		case *pbsubstreams.Module_KindMap_:
			outputType := i.KindMap.OutputType
			if !r.skipModuleOutputTypeValidation {
				if !strings.HasPrefix(outputType, "proto:") {
					return fmt.Errorf("module %q incorrect outputTyupe %q valueType must be a proto Message", mod.Name, outputType)
				}
			}
		case *pbsubstreams.Module_KindStore_:
			valueType := i.KindStore.ValueType
			if !r.skipModuleOutputTypeValidation {
				if strings.HasPrefix(valueType, "proto:") {
					// any store with a prototype is considered valid
				} else if !storeValidTypes[valueType] {
					return fmt.Errorf("module %q: invalid valueType %q", mod.Name, valueType)
				}
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

	if mods == nil {
		return fmt.Errorf("no modules found in request")
	}

	for _, binary := range mods.Binaries {
		sumCode += len(binary.Content)
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
			case *pbsubstreams.Module_Input_Params_:
				if idx != 0 {
					return fmt.Errorf("module %q: input %d: params must be first input", mod.Name, idx)
				}
			case *pbsubstreams.Module_Input_Source_:
				if i.Source.Type == "" {
					return fmt.Errorf("module %q: source type empty", mod.Name)
				}
			case *pbsubstreams.Module_Input_Map_:
				seekMod := i.Map.ModuleName
				var found bool
				for _, mod2 := range mods.Modules {
					if mod2.Name == seekMod {
						found = true
						if _, ok := mod2.Kind.(*pbsubstreams.Module_KindMap_); !ok {
							return fmt.Errorf("module %q: input %d: referenced module %q not of 'map' kind", mod.Name, idx, seekMod)
						}
					}
				}
				if !found {
					return fmt.Errorf("module %q: map input named %q not found", mod.Name, seekMod)
				}
			case *pbsubstreams.Module_Input_Store_:
				seekMod := i.Store.ModuleName
				var found bool
				for _, mod2 := range mods.Modules {
					if mod2.Name == seekMod {
						found = true
						if _, ok := mod2.Kind.(*pbsubstreams.Module_KindStore_); !ok {
							return fmt.Errorf("module %q: input %d: referenced module %q not of 'store' kind", mod.Name, idx, seekMod)
						}
					}
				}
				if !found {
					return fmt.Errorf("module %q: store input named %q not found", mod.Name, seekMod)
				}

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

func LoadManifestFile(inputPath string) (*Manifest, error) {
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

	// Allow environment variables in `imports` element
	for i, moduleImport := range m.Imports {
		m.Imports[i][1] = os.ExpandEnv(moduleImport[1])
	}

	// Allow environment variables in `protobuf.importPaths` element
	for i := range m.Protobuf.ImportPaths {
		m.Protobuf.ImportPaths[i] = os.ExpandEnv(m.Protobuf.ImportPaths[i])
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
		case ModuleKindStore:
			if err := validateStoreBuilder(s); err != nil {
				return nil, fmt.Errorf("stream %q: %w", s.Name, err)
			}

		default:
			return nil, fmt.Errorf("stream %q: invalid kind %q", s.Name, s.Kind)
		}
		for idx, input := range s.Inputs {
			if err := input.parse(); err != nil {
				return nil, fmt.Errorf("module %q: invalid input [%d]: %w", s.Name, idx, err)
			}
		}
	}

	return m, nil
}

func loadImports(pkg *pbsubstreams.Package, manif *Manifest) error {
	for _, kv := range manif.Imports {
		importName := kv[0]
		importPath := manif.resolvePath(kv[1])

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
		for idx, inputIface := range mod.Inputs {
			switch input := inputIface.Input.(type) {
			case *pbsubstreams.Module_Input_Source_:
			case *pbsubstreams.Module_Input_Store_:
				input.Store.ModuleName = prefix + PrefixSeparator + input.Store.ModuleName
			case *pbsubstreams.Module_Input_Map_:
				input.Map.ModuleName = prefix + PrefixSeparator + input.Map.ModuleName
			case *pbsubstreams.Module_Input_Params_:
			default:
				panic(fmt.Sprintf("module %q: input index %d: unsupported module input type %s", mod.Name, idx, inputIface.Input))
			}
		}
	}
}

// mergeAndReindexPackages consumes the `src` Package into `dest`, and
// modifies `src`.
func reindexAndMergePackage(src, dest *pbsubstreams.Package) {
	newBasePackageIndex := len(dest.PackageMeta)
	newBaseBinariesIndex := len(dest.Modules.Binaries)

	for _, modMeta := range src.ModuleMeta {
		modMeta.PackageIndex += uint64(newBasePackageIndex)
	}
	for _, mod := range src.Modules.Modules {
		mod.BinaryIndex += uint32(newBaseBinariesIndex)
	}
	dest.Modules.Modules = append(dest.Modules.Modules, src.Modules.Modules...)
	dest.Modules.Binaries = append(dest.Modules.Binaries, src.Modules.Binaries...)
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
func (r *Reader) manifestToPkg(m *Manifest) (*pbsubstreams.Package, []*desc.FileDescriptor, error) {
	pkg, err := r.convertToPkg(m)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to convert manifest to pkg: %w", err)
	}

	protoDefinitions, err := loadProtobufs(pkg, m)
	if err != nil {
		return nil, nil, fmt.Errorf("error loading protobuf: %w", err)
	}

	if err := loadImports(pkg, m); err != nil {
		return nil, nil, fmt.Errorf("error loading imports: %w", err)
	}

	return pkg, protoDefinitions, nil
}

func (r *Reader) convertToPkg(m *Manifest) (pkg *pbsubstreams.Package, err error) {
	pkgMeta := &pbsubstreams.PackageMetadata{
		Version: m.Package.Version,
		Url:     m.Package.URL,
		Name:    m.Package.Name,
		Doc:     m.Package.Doc,
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

var storeValidTypes = map[string]bool{
	"bigint":     true,
	"int64":      true,
	"float64":    true,
	"bigdecimal": true,
	"bigfloat":   true,
	"bytes":      true,
	"string":     true,
	"proto":      true,
}
