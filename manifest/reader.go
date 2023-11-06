package manifest

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	ipfs "github.com/ipfs/go-ipfs-api"
	"github.com/jhump/protoreflect/dynamic"
	"github.com/streamingfast/dstore"
	"golang.org/x/mod/semver"
	"google.golang.org/protobuf/proto"

	"github.com/jhump/protoreflect/desc"
	"github.com/streamingfast/cli"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"go.uber.org/zap"
	"gopkg.in/yaml.v2"
	yaml3 "gopkg.in/yaml.v3"
)

var IPFSURL string
var IPFSTimeout time.Duration
var httpClient = &http.Client{
	Timeout: 30 * time.Second,
}

type Option func(r *Reader) *Reader

func SkipSourceCodeReader() Option {
	return func(r *Reader) *Reader {
		r.skipSourceCodeImportValidation = true
		return r
	}
}

func SkipModuleOutputTypeValidationReader() Option {
	return func(r *Reader) *Reader {
		r.skipModuleOutputTypeValidation = true
		return r
	}
}

func SkipPackageValidationReader() Option {
	return func(r *Reader) *Reader {
		r.skipPackageValidation = true
		return r
	}
}

func WithCollectProtoDefinitions(f func(protoDefinitions []*desc.FileDescriptor)) Option {
	return func(r *Reader) *Reader {
		r.collectProtoDefinitionsFunc = f
		return r
	}
}

func hasRemotePrefix(in string) bool {
	for _, prefix := range []string{"https://", "http://", "ipfs://", "gs://", "s3://", "az://"} {
		if strings.HasPrefix(in, prefix) {
			return true
		}
	}

	return false
}

type Reader struct {
	currentData []byte

	originalInput string
	currentInput  string

	workingDir string

	pkg       *pbsubstreams.Package
	overrides []*ConfigurationOverride

	// cached values
	protoDefinitions         []*desc.FileDescriptor
	sinkConfigJSON           string
	sinkConfigDynamicMessage *dynamic.Message

	collectProtoDefinitionsFunc func(protoDefinitions []*desc.FileDescriptor)

	//options
	skipSourceCodeImportValidation bool
	skipModuleOutputTypeValidation bool
	skipPackageValidation          bool
}

func NewReader(input string, opts ...Option) (*Reader, error) {
	workingDir, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("unable to get working dir: %w", err)
	}

	return newReader(input, workingDir, opts...)
}

func MustNewReader(input string, opts ...Option) *Reader {
	r, err := NewReader(input)
	if err != nil {
		panic(err)
	}

	return r
}

func newReader(input, workingDir string, opts ...Option) (*Reader, error) {
	r := &Reader{
		originalInput: input,
		workingDir:    workingDir,
	}

	if err := r.resolveInputPath(); err != nil {
		return nil, err
	}

	for _, opt := range opts {
		opt(r)
	}

	return r, nil
}

func (r *Reader) Read() (*pbsubstreams.Package, error) {
	return r.resolvePkg()
}

func (r *Reader) MustRead() *pbsubstreams.Package {
	pkg, err := r.Read()
	if err != nil {
		panic(err)
	}

	return pkg
}

func (r *Reader) read() error {
	input := r.currentInput
	if r.IsRemotePackage(input) {
		return r.readRemote(input)
	}

	return r.readLocal(input)
}

func (r *Reader) readRemote(input string) error {
	u, err := url.Parse(input)
	if err != nil {
		return fmt.Errorf("unable to parse url %q: %w", input, err)
	}

	if u.Scheme == "gs" || u.Scheme == "s3" || u.Scheme == "az" {
		return r.readFromStore(input)
	}

	if u.Scheme == "ipfs" {
		return r.readFromIPFS(u.Host)
	}

	return r.readFromHttp(input)
}

func (r *Reader) readFromHttp(input string) error {
	resp, err := httpClient.Get(input)
	if err != nil {
		return fmt.Errorf("error downloading %q: %w", input, err)
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("error reading %q: %w", input, err)
	}

	r.currentData = b
	return nil
}

func (r *Reader) readFromStore(input string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	b, err := dstore.ReadObject(ctx, input)
	if err != nil {
		return fmt.Errorf("error reading %q: %w", input, err)
	}

	r.currentData = b
	return nil
}

func (r *Reader) readFromIPFS(input string) error {
	readIPFSContent := func(hash string, sh *ipfs.Shell) ([]byte, error) {
		readCloser, err := sh.Cat(hash)
		if err != nil {
			return nil, err
		}
		defer readCloser.Close()
		return io.ReadAll(readCloser)
	}

	sh := ipfs.NewShell(IPFSURL)
	sh.SetTimeout(IPFSTimeout)

	b, err := readIPFSContent(input, sh)
	if err != nil {
		return fmt.Errorf("unable to read ipfs content %q: %w", input, err)
	}

	r.currentData = b

	if r.isOverride() {
		return nil
	}

	type subgraphManifest struct {
		DataSources []struct {
			Kind   string `yaml:"kind"`
			Source struct {
				Package struct {
					File map[string]string `yaml:"file"`
				} `yaml:"package"`
			} `yaml:"source"`
		} `yaml:"dataSources"`
	}

	manifest := &subgraphManifest{}
	err = yaml.Unmarshal(b, manifest)
	if err != nil || len(manifest.DataSources) == 0 {
		// not a valid manifest, it is probably the spkg itself
		return nil
	}

	r.currentData = nil

	if manifest.DataSources[0].Kind != "substreams" {
		return fmt.Errorf("given ipfs hash is not a substreams-based subgraph")
	}

	var spkgHash string
	if len(manifest.DataSources) > 0 && manifest.DataSources[0].Source.Package.File != nil {
		spkgHash = manifest.DataSources[0].Source.Package.File["/"]
	}

	if spkgHash == "" {
		return fmt.Errorf("no spkg hash found in manifest")
	}

	b, err = readIPFSContent(spkgHash, sh)
	if err != nil {
		return fmt.Errorf("unable to read spkg from ipfs %q: %w", spkgHash, err)
	}

	r.currentData = b
	return nil
}

func (r *Reader) readLocal(input string) error {
	input = r.currentInput

	b, err := os.ReadFile(input)
	if err != nil {
		return fmt.Errorf("unable to read file %q: %w", input, err)
	}

	r.currentData = b
	return nil
}

func (r *Reader) resolveInputPath() error {
	input := r.originalInput
	if r.IsRemotePackage(input) {
		r.currentInput = input
		return nil
	}

	if input == "" {
		r.currentInput = filepath.Join(r.workingDir, "substreams.yaml")
		return nil
	}

	stat, err := os.Stat(input)
	if err != nil {
		return fmt.Errorf("unable to stat input file %q: %w", input, err)
	}

	if stat.IsDir() {
		r.currentInput = filepath.Join(input, "substreams.yaml")
		return nil
	}

	r.currentInput = input

	return nil
}

func (r *Reader) isOverride() bool {
	if r.currentData == nil {
		return false
	}
	return bytes.Contains(r.currentData, []byte("deriveFrom:"))
}

func (r *Reader) getPkg() (*pbsubstreams.Package, error) {
	if r.currentData == nil {
		return nil, fmt.Errorf("no result available")
	}

	if r.isOverride() {
		return nil, fmt.Errorf("cannot get package from override")
	}

	if strings.HasSuffix(r.currentInput, ".yaml") || strings.HasSuffix(r.currentInput, ".yml") {
		manif := &Manifest{}
		decoder := yaml3.NewDecoder(bytes.NewReader(r.currentData))
		decoder.KnownFields(true)

		if err := decoder.Decode(&manif); err != nil {
			return nil, fmt.Errorf("unable to unmarshal manifest: %w", err)
		}

		pkg, err := r.newPkgFromManifest(manif)
		if err != nil {
			return nil, fmt.Errorf("unable to convert manifest to package: %w", err)
		}

		if err := r.validate(pkg); err != nil {
			return nil, fmt.Errorf("failed validation: %w", err)
		}
		return pkg, nil
	}

	pkg := &pbsubstreams.Package{}
	err := proto.Unmarshal(r.currentData, pkg)
	if err != nil {
		return nil, fmt.Errorf("unable to unmarshal package: %w", err)
	}

	if err := r.validate(pkg); err != nil {
		return nil, fmt.Errorf("failed validation: %w", err)
	}

	return pkg, nil
}

func (r *Reader) validate(pkg *pbsubstreams.Package) error {
	if !r.skipPackageValidation {
		if err := r.validatePackage(pkg); err != nil {
			return fmt.Errorf("package validation failed: %w", err)
		}
	}

	if err := ValidateModules(pkg.Modules); err != nil {
		return fmt.Errorf("module validation failed: %w", err)
	}
	return nil
}

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

		inputSeen := map[string]bool{}
		for _, in := range mod.Inputs {
			_ = in
			s := duplicateStringInput(in)
			if inputSeen[s] {
				return fmt.Errorf("module %q: duplicate input %q", mod.Name, s)
			}
			inputSeen[s] = true
			// TODO: do more proto ref checking for those inputs listed
		}
	}

	// TODO: Loop through inputs, outputs, and check that all internal proto references are satisfied by the FileDescriptors

	if pkg.SinkModule != "" {
		var found bool
		for _, mod := range pkg.Modules.Modules {
			if mod.Name == pkg.SinkModule {
				found = true
			}
		}
		if !found {
			return fmt.Errorf("sink: module: module %q not found in package", pkg.SinkModule)
		}
	}

	return nil
}

func (r *Reader) newPkgFromManifest(manif *Manifest) (*pbsubstreams.Package, error) {
	converter := newManifestConverter(r.currentInput, r.skipSourceCodeImportValidation)
	pkg, descriptors, dynMessage, err := converter.Convert(manif)
	if err != nil {
		return nil, err
	}
	r.sinkConfigDynamicMessage = dynMessage

	if r.collectProtoDefinitionsFunc != nil {
		r.collectProtoDefinitionsFunc(descriptors)
	}
	r.protoDefinitions = descriptors
	return pkg, nil
}

func (r *Reader) getOverride() (*ConfigurationOverride, error) {
	if r.currentData == nil {
		return nil, fmt.Errorf("no result available")
	}

	if !r.isOverride() {
		return nil, fmt.Errorf("not an override")
	}

	override := &ConfigurationOverride{}
	err := yaml.Unmarshal(r.currentData, override)
	if err != nil {
		return nil, fmt.Errorf("unable to unmarshal override: %w", err)
	}

	return override, nil
}

func (r *Reader) resolvePkg() (*pbsubstreams.Package, error) {
	if r.pkg != nil {
		return r.pkg, nil
	}

	err := r.read()
	if err != nil {
		return nil, err
	}

	if r.isOverride() {
		or, err := r.getOverride()
		if err != nil {
			return nil, fmt.Errorf("unable to get override: %w", err)
		}
		r.overrides = append(r.overrides, or)
		r.currentInput = or.DeriveFrom

		return r.resolvePkg()
	}

	pkg, err := r.getPkg()
	if err != nil {
		return nil, fmt.Errorf("unable to get package: %w", err)
	}

	//reverse order r.overrides to be able to squash them in the right order
	for i, j := 0, len(r.overrides)-1; i < j; i, j = i+1, j-1 {
		r.overrides[i], r.overrides[j] = r.overrides[j], r.overrides[i]
	}

	mergedOverride := mergeOverrides(r.overrides...)
	if err := applyOverride(pkg, mergedOverride); err != nil {
		return nil, fmt.Errorf("applying override: %w", err)
	}

	r.pkg = pkg
	return pkg, nil
}

// IsRemotePackage determines if reader's input to read the manifest is a remote file accessible over
// HTTP/HTTPS, Google Cloud Storage, S3 or Azure Storage.
func (r *Reader) IsRemotePackage(input string) bool {
	return hasRemotePrefix(input)
}

// IsLocalManifest determines if reader's input to read the manifest is a local manifest file, which is determined
// by ensure it's not a remote package and if the file end with `.yaml` or `.yml`.
func (r *Reader) IsLocalManifest() bool {
	if r.IsRemotePackage(r.currentInput) {
		return false
	}

	return strings.HasSuffix(r.currentInput, ".yaml") || strings.HasSuffix(r.currentInput, ".yml")
}

// IsLikelyManifestInput determines if the input is likely a manifest input, which is determined
// by checking:
//   - If the input starts with remote prefix ("https://", "http://", "ipfs://", "gs://", "s3://", "az://")
//   - If the input ends with `.yaml`
//   - If the input is a directory (we check for path separator)
func IsLikelyManifestInput(in string) bool {
	if hasRemotePrefix(in) {
		return true
	}

	if strings.HasSuffix(in, ".yaml") {
		return true
	}

	if strings.Contains(in, string(os.PathSeparator)) {
		return true
	}

	return cli.DirectoryExists(in) || cli.FileExists(in)
}

func duplicateStringInput(in *pbsubstreams.Module_Input) string {
	if in == nil {
		return ""
	}
	switch put := in.Input.(type) {
	case *pbsubstreams.Module_Input_Source_:
		return fmt.Sprintf("source: %s", put.Source.Type)
	case *pbsubstreams.Module_Input_Map_:
		return fmt.Sprintf("map: %s", put.Map.ModuleName)
	case *pbsubstreams.Module_Input_Store_:
		return fmt.Sprintf("store: %s, mode: %s", put.Store.ModuleName, strings.ToLower(put.Store.Mode.String()))
	case *pbsubstreams.Module_Input_Params_:
		return "params"
	default:
		return ""
	}
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

func LoadManifestFile(inputPath, workingDir string) (*Manifest, error) {
	m, err := decodeYamlManifestFromFile(inputPath, workingDir)
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

		subpkgReader := MustNewReader(importPath)
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

var PNGHeader = []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}
var JPGHeader = []byte{0xff, 0xd8, 0xff}
var WebPHeader = []byte{0x52, 0x49, 0x46, 0x46}

func loadImage(pkg *pbsubstreams.Package, manif *Manifest) error {
	path := manif.resolvePath(manif.Package.Image)
	img, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	maxSize := 2 * 1024 * 1024
	if len(img) > maxSize {
		return fmt.Errorf("image %q is too big: %d bytes. (Max: %d bytes)", path, len(img), maxSize)
	}
	if len(img) < 16 { // prevent error on magic header check, also 16 bytes is way too small.
		return fmt.Errorf("invalid image file: too small (%d bytes)", len(img))
	}

	switch {
	case len(img) > 8 && bytes.Equal(img[0:8], PNGHeader):
	case bytes.Equal(img[0:3], JPGHeader):
	case bytes.Equal(img[0:4], WebPHeader):
	default:
		return fmt.Errorf("Unsupported file format for %q. Only JPEG, PNG and WebP images are supported", path)
	}

	pkg.Image = img
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
			zlog.Debug("skipping proto file already seen", zap.String("proto_file", *file.Name))
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
