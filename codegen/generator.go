package codegen

import (
	_ "embed"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/jhump/protoreflect/desc"
	"github.com/streamingfast/substreams/manifest"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/runtime/protoimpl"
	"google.golang.org/protobuf/types/descriptorpb"
)

var E_RUST_MODULE = &protoimpl.ExtensionInfo{
	ExtendedType:  (*descriptorpb.FileOptions)(nil),
	ExtensionType: (*string)(nil),
	Field:         56781,
	Name:          "rust_module",
	Tag:           "bytes",
}

func init() {
	err := protoregistry.GlobalTypes.RegisterExtension(E_RUST_MODULE)
	if err != nil {
		panic(fmt.Errorf("registering proto extension rust_module: %w", err))
	}
}

//go:embed templates/lib.gotmpl
var tplLibRs string

//go:embed templates/externs.gotmpl
var tplExterns string

//go:embed templates/substreams.gotmpl
var tplSubstreams string

//go:embed templates/mod.gotmpl
var tplMod string

//go:embed templates/pb_mod.gotmpl
var tplPbMod string

//go:embed templates/buildsh.gotmpl
var tplBuildSh string

//go:embed templates/cargotoml.gotmpl
var tplCargoToml string

//go:embed templates/manifestyaml.gotmpl
var tplManifestYaml string

//go:embed templates/rusttoolchain.gotmpl
var tplRustToolchain string

var StoreType = map[string]string{
	"bytes":      "Raw",
	"string":     "String",
	"bigint":     "BigInt",
	"bigdecimal": "BigDecimal",
	"bigfloat":   "BigDecimal",
	"int64":      "Int64",
	"i64":        "Int64",
	"float64":    "Float64",
	"boolean":    "bool",
}

func maybeTranslateType(t string) string {
	u, ok := StoreType[t]
	if !ok {
		return t
	}
	return u
}

var UpdatePoliciesMap = map[string]string{
	"":                                  "Unset",
	manifest.UpdatePolicySet:            "Set",
	manifest.UpdatePolicySetIfNotExists: "SetIfNotExists",
	manifest.UpdatePolicyAdd:            "Add",
	manifest.UpdatePolicyMin:            "Min",
	manifest.UpdatePolicyMax:            "Max",
	manifest.UpdatePolicyAppend:         "Append",
}

type Generator struct {
	pkg              *pbsubstreams.Package
	manifest         *manifest.Manifest
	srcPath          string
	protoDefinitions []*desc.FileDescriptor
	writer           io.Writer
	engine           *Engine
}

func NewGenerator(pkg *pbsubstreams.Package, manifest *manifest.Manifest, protoDefinitions []*desc.FileDescriptor, srcPath string) *Generator {
	engine := &Engine{Manifest: manifest}
	utils["getEngine"] = engine.GetEngine

	return &Generator{
		pkg:              pkg,
		manifest:         manifest,
		srcPath:          srcPath,
		protoDefinitions: protoDefinitions,
		engine:           engine,
	}
}

func (g *Generator) Generate() (err error) {
	if _, err := os.Stat(g.srcPath); errors.Is(err, os.ErrNotExist) {
		fmt.Printf("Creating missing %q folder\n", g.srcPath)
		if err := os.MkdirAll(g.srcPath, os.ModePerm); err != nil {
			return fmt.Errorf("creating src directory %v: %w", g.srcPath, err)
		}
	}
	fmt.Printf("Generating files in %q\n", g.srcPath)

	generatedFolder := filepath.Join(g.srcPath, "generated")
	if err := os.MkdirAll(generatedFolder, os.ModePerm); err != nil {
		return fmt.Errorf("creating generated directory %v: %w", g.srcPath, err)
	}

	pbFolder := filepath.Join(g.srcPath, "pb")
	if err := os.MkdirAll(pbFolder, os.ModePerm); err != nil {
		return fmt.Errorf("creating pb directory %v: %w", g.srcPath, err)
	}

	protoGenerator := NewProtoGenerator(pbFolder, nil, false)
	err = protoGenerator.GenerateProto(g.pkg)
	if err != nil {
		return fmt.Errorf("generating protobuf code: %w", err)
	}

	err = generate("externs", tplExterns, g.engine, filepath.Join(generatedFolder, "externs.rs"))
	if err != nil {
		return fmt.Errorf("generating externs.rs: %w", err)
	}
	fmt.Println("Externs generated")

	err = generate("Substream", tplSubstreams, g.engine, filepath.Join(generatedFolder, "substreams.rs"))
	if err != nil {
		return fmt.Errorf("generating substreams.rs: %w", err)
	}

	err = generate("mod", tplMod, g.engine, filepath.Join(generatedFolder, "mod.rs"))
	if err != nil {
		return fmt.Errorf("generating mod.rs: %w", err)
	}
	fmt.Println("Substreams Trait and base struct generated")

	pbModFilePath := filepath.Join(filepath.Join(pbFolder, "mod.rs"))
	if _, err := os.Stat(pbModFilePath); errors.Is(err, os.ErrNotExist) {
		err = generate("pb/mod", tplPbMod, protoPackages(g.protoDefinitions), pbModFilePath)
		if err != nil {
			return fmt.Errorf("generating pb/mod.rs: %w", err)
		}
		fmt.Println("Protobuf pb/mod.rs generated")
	}

	libFilePath := filepath.Join(g.srcPath, "lib.rs")
	if _, err := os.Stat(libFilePath); errors.Is(err, os.ErrNotExist) {
		fmt.Printf("Generating src/lib.rs\n")
		err = generate("lib", tplLibRs, g.engine, filepath.Join(g.srcPath, "lib.rs"))
		if err != nil {
			return fmt.Errorf("generating lib.rs: %w", err)
		}
	} else {
		fmt.Printf("Skipping existing src/lib.rs\n")
	}

	return nil
}

func protoPackages(protoDefinitions []*desc.FileDescriptor) map[string]string {
	protoPackages := map[string]string{}
	for _, definition := range protoDefinitions {
		p := definition.GetPackage()
		options := definition.GetOptions().(*descriptorpb.FileOptions)
		rustModule := proto.GetExtension(options, E_RUST_MODULE).(string)
		if rustModule == "" {
			rustModule = strings.ReplaceAll(p, ".", "_")
		}
		protoPackages[p] = rustModule
	}
	return protoPackages
}

type GenerationOptions func(options *generateOptions)
type generateOptions struct {
	w io.Writer
}

func WithTestWriter(w io.Writer) GenerationOptions {
	return func(options *generateOptions) {
		options.w = w
	}
}

func generate(name, tpl string, data any, outputFile string, options ...GenerationOptions) (err error) {
	var w io.Writer

	opts := &generateOptions{}
	for _, option := range options {
		option(opts)
	}

	if opts.w != nil {
		w = opts.w
	} else {
		w, err = os.Create(outputFile)
		if err != nil {
			return fmt.Errorf("creating file %s: %w", outputFile, err)
		}
	}

	tmpl, err := template.New(name).Funcs(utils).Parse(tpl)
	if err != nil {
		return fmt.Errorf("parsing %q template: %w", name, err)
	}

	err = tmpl.Execute(
		w,
		data,
	)
	if err != nil {
		return fmt.Errorf("executing %q template: %w", name, err)
	}

	return nil
}

var utils = map[string]any{
	"contains":  strings.Contains,
	"hasPrefix": strings.HasPrefix,
	"hasSuffix": strings.HasSuffix,
}

type Engine struct {
	Manifest *manifest.Manifest
}

func (e *Engine) GetEngine() *Engine {
	return e
}

func (e *Engine) MustModule(moduleName string) *manifest.Module {
	for _, module := range e.Manifest.Modules {
		if module.Name == moduleName {
			return module
		}
	}
	panic(fmt.Sprintf("MustModule %q not found", moduleName))
}

func (e *Engine) moduleOutputForName(moduleName string) (string, error) {
	//todo: call MustModule ...
	for _, module := range e.Manifest.Modules {
		if module.Name == moduleName {
			return module.Output.Type, nil
		}
	}
	return "", fmt.Errorf("MustModule %q not found", moduleName)
}

func (e *Engine) FunctionSignature(module *manifest.Module) (*FunctionSignature, error) {
	switch module.Kind {
	case manifest.ModuleKindMap:
		return e.mapFunctionSignature(module)
	case manifest.ModuleKindStore:
		return e.storeFunctionSignature(module)
	default:
		return nil, fmt.Errorf("unknown must module kind: %T", module.Kind)
	}
}

func (e *Engine) mapFunctionSignature(module *manifest.Module) (*FunctionSignature, error) {
	inputs, err := e.ModuleArgument(module.Inputs)
	if err != nil {
		return nil, fmt.Errorf("generating must module intputs: %w", err)
	}

	outType := module.Output.Type
	if strings.HasPrefix(outType, "proto:") {
		outType = mustTransformProtoType(outType, e.Manifest)
	}

	fn := NewFunctionSignature(module.Name, "map", maybeTranslateType(outType), "", inputs)

	return fn, nil
}

func (e *Engine) storeFunctionSignature(module *manifest.Module) (*FunctionSignature, error) {
	arguments, err := e.ModuleArgument(module.Inputs)
	if err != nil {
		return nil, fmt.Errorf("generating MustModule intputs: %w", err)
	}

	fn := NewFunctionSignature(module.Name, "store", "", module.UpdatePolicy, arguments)

	return fn, nil
}

func (e *Engine) ModuleArgument(inputs []*manifest.Input) (Arguments, error) {
	var out Arguments
	for _, input := range inputs {
		switch {
		case input.IsMap():
			inputType, err := e.moduleOutputForName(input.Map)
			if err != nil {
				return nil, fmt.Errorf("getting map type: %w", err)
			}
			if strings.HasPrefix(inputType, "proto:") {
				inputType = mustTransformProtoType(inputType, e.Manifest)
			}
			out = append(out, NewArgument(input.Map, inputType, input))
		case input.IsStore():
			inputType := e.MustModule(input.Store).ValueType
			if strings.HasPrefix(inputType, "proto:") {
				inputType = mustTransformProtoType(inputType, e.Manifest)
			}
			out = append(out, NewArgument(input.Store, inputType, input))
		case input.IsSource():
			inputType := mustTransformProtoType(input.Source, e.Manifest)

			parts := strings.Split(input.Source, ".")
			name := parts[len(parts)-1]
			name = strings.ToLower(name)

			out = append(out, NewArgument(name, inputType, input))
		case input.IsParams():
			inputType := strings.Trim(input.Params, " ")
			out = append(out, NewArgument("params", inputType, input))
		default:
			return nil, fmt.Errorf("unknown MustModule kind: %T", input)
		}
	}
	return out, nil
}

func (e *Engine) ReadableStoreType(store *manifest.Module, input *manifest.Input) string {
	t := store.ValueType
	p := store.UpdatePolicy

	//TODO(colin): split out deltas code into a separate function
	if input.Mode == "deltas" {
		if strings.HasPrefix(t, "proto") {
			t = mustTransformProtoType(t, e.Manifest)
			return fmt.Sprintf("substreams::store::Deltas<substreams::store::DeltaProto<%s>>", t)
		}
		if p == manifest.UpdatePolicyAppend {
			return fmt.Sprintf("substreams::store::Deltas<substreams::store::DeltaArray<%s>>", maybeTranslateType(t))
		}

		t = maybeTranslateType(t)
		return fmt.Sprintf("substreams::store::Deltas<substreams::store::Delta%s>", t)
	}

	if strings.HasPrefix(t, "proto") {
		t = mustTransformProtoType(t, e.Manifest)
		return fmt.Sprintf("substreams::store::StoreGetProto<%s>", t)
	}

	if p == manifest.UpdatePolicyAppend {
		return fmt.Sprintf("substreams::store::StoreGetRaw")
	}

	t = maybeTranslateType(t)
	return fmt.Sprintf("substreams::store::StoreGet%s", t)
}

func (e *Engine) WritableStoreType(store *manifest.Module) string {
	t := store.ValueType
	p := store.UpdatePolicy

	if p == manifest.UpdatePolicyAppend {
		return fmt.Sprintf("substreams::store::StoreAppend<%s>", maybeTranslateType(t))
	}

	p = UpdatePoliciesMap[p]
	if strings.HasPrefix(t, "proto") {
		t = mustTransformProtoType(t, e.Manifest)
		return fmt.Sprintf("substreams::store::Store%sProto<%s>", p, t)
	}

	return fmt.Sprintf("substreams::store::Store%s%s", p, maybeTranslateType(t))
}

func (e *Engine) WritableStoreDeclaration(store *manifest.Module) string {
	t := store.ValueType
	p := store.UpdatePolicy

	if p == manifest.UpdatePolicyAppend {
		return fmt.Sprintf("let store: substreams::store::StoreAppend<%s> = substreams::store::StoreAppend::new();", maybeTranslateType(t))
	}

	p = UpdatePoliciesMap[p]

	if strings.HasPrefix(t, "proto") {
		t = mustTransformProtoType(t, e.Manifest)
		return fmt.Sprintf("let store: substreams::store::Store%sProto<%s> = substreams::store::StoreSetProto::new();", p, t)
	}
	t = maybeTranslateType(t)
	return fmt.Sprintf("let store: substreams::store::Store%s%s = substreams::store::Store%s%s::new();", p, t, p, t)
}

func (e *Engine) ReadableStoreDeclaration(name string, store *manifest.Module, input *manifest.Input) string {
	t := store.ValueType
	p := store.UpdatePolicy
	isProto := strings.HasPrefix(t, "proto")
	if isProto {
		t = mustTransformProtoType(t, e.Manifest)
	}

	if input.Mode == "deltas" {

		raw := fmt.Sprintf("let raw_%s_deltas = substreams::proto::decode_ptr::<substreams::pb::substreams::StoreDeltas>(%s_deltas_ptr, %s_deltas_len).unwrap().deltas;", name, name, name)
		delta := fmt.Sprintf("let %s_deltas: substreams::store::Deltas<substreams::store::Delta%s> = substreams::store::Deltas::new(raw_%s_deltas);", name, maybeTranslateType(t), name)

		if p == manifest.UpdatePolicyAppend {
			delta = fmt.Sprintf("let %s_deltas: substreams::store::Deltas<substreams::store::DeltaArray<%s>> = substreams::store::Deltas::new(raw_%s_deltas);", name, maybeTranslateType(t), name)
		}

		if isProto {
			delta = fmt.Sprintf("let %s_deltas: substreams::store::Deltas<substreams::store::DeltaProto<%s>> = substreams::store::Deltas::new(raw_%s_deltas);", name, t, name)
		}
		return raw + "\n\t\t" + delta
	}

	if isProto {
		return fmt.Sprintf("let %s: substreams::store::StoreGetProto<%s>  = substreams::store::StoreGetProto::new(%s_ptr);", name, t, name)
	}

	t = maybeTranslateType(t)
	if p == manifest.UpdatePolicyAppend {
		return fmt.Sprintf("let %s: substreams::store::StoreGetRaw = substreams::store::StoreGetRaw::new(%s_ptr);", name, name)
	}

	return fmt.Sprintf("let %s: substreams::store::StoreGet%s = substreams::store::StoreGet%s::new(%s_ptr);", name, t, t, name)

}

func mustTransformProtoType(t string, manif *manifest.Manifest) string {
	t = strings.TrimPrefix(t, "proto:")

	parts := strings.Split(t, ".")
	entityName := parts[len(parts)-1]

	if len(parts) == 1 {
		return entityName
	}
	protoPackage := strings.Join(parts[:len(parts)-1], ".")
	if resolved, ok := manif.Binaries["default"].ProtoPackageMapping[protoPackage]; ok {
		protoPackage = resolved + "::" + parts[len(parts)-1]
		return protoPackage
	}

	panic(fmt.Errorf("missing binaries.default.protoPackageMapping value for %q", protoPackage))
}

type FunctionSignature struct {
	Name        string
	Type        string
	OutputType  string
	StorePolicy string
	Arguments   Arguments
}

func NewFunctionSignature(name string, t string, outType string, storePolicy string, arguments Arguments) *FunctionSignature {
	return &FunctionSignature{
		Name:        name,
		Type:        t,
		OutputType:  maybeTranslateType(outType),
		StorePolicy: storePolicy,
		Arguments:   arguments,
	}
}

type Arguments []*Argument

type Argument struct {
	Name        string
	Type        string
	ModuleInput *manifest.Input
}

func NewArgument(name string, argType string, moduleInput *manifest.Input) *Argument {
	return &Argument{
		Name:        name,
		Type:        maybeTranslateType(argType),
		ModuleInput: moduleInput,
	}
}
