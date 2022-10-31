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
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

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

const EthereumBlockManifest = "sf.ethereum.type.v2.Block"
const EthereumBlockRust = "substreams_ethereum::pb::eth::v2::Block"

const SubstreamsClock = "sf.substreams.v1.Clock"
const SubstreamsClockRust = "substreams::pb::substreams::Clock"

var StoreType = map[string]string{
	"bytes":      "Raw",
	"string":     "String",
	"bigint":     "BigInt",
	"bigdecimal": "BigDecimal",
	"bigfloat":   "BigDecimal",
	"int64":      "I64",
	"i64":        "I64",
	"float64":    "Float64",
}

var UpdatePoliciesMap = map[string]string{
	"UPDATE_POLICY_UNSET":             "Unset",
	"UPDATE_POLICY_SET":               "Set",
	"UPDATE_POLICY_SET_IF_NOT_EXISTS": "SetIfNotExist",
	"UPDATE_POLICY_ADD":               "Add",
	"UPDATE_POLICY_MIN":               "Min",
	"UPDATE_POLICY_MAX":               "Max",
	"UPDATE_POLICY_APPEND":            "Append",
}

type Generator struct {
	pkg              *pbsubstreams.Package
	srcPath          string
	protoDefinitions []*desc.FileDescriptor
	writer           io.Writer
	engine           *Engine
}

func NewGenerator(pkg *pbsubstreams.Package, protoDefinitions []*desc.FileDescriptor, srcPath string) *Generator {
	engine := &Engine{Package: pkg}
	utils["getEngine"] = engine.GetEngine
	return &Generator{
		pkg:              pkg,
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

	protoGenerator := NewProtoGenerator(pbFolder, nil)
	err = protoGenerator.Generate(g.pkg)
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

	protoPackages := map[string]string{}
	for _, definition := range g.protoDefinitions {
		p := definition.GetPackage()
		protoPackages[p] = strings.ReplaceAll(p, ".", "_")
	}

	err = generate("pb/mod", tplPbMod, protoPackages, filepath.Join(pbFolder, "mod.rs"))
	if err != nil {
		return fmt.Errorf("generating pb/mod.rs: %w", err)
	}
	fmt.Println("Protobuf pb/mod.rs generated")

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
	"contains":                 strings.Contains,
	"hasPrefix":                strings.HasPrefix,
	"hasSuffix":                strings.HasSuffix,
	"isDelta":                  IsDelta,
	"isStoreModule":            IsStoreModule,
	"isMapModule":              IsMapModule,
	"isStoreInput":             IsStoreInput,
	"isMapInput":               IsMapInput,
	"writableStoreDeclaration": WritableStoreDeclaration,
	"writableStoreType":        WritableStoreType,
	"readableStoreDeclaration": ReadableStoreDeclaration,
	"readableStoreType":        ReadableStoreType,
}

type Engine struct {
	Package *pbsubstreams.Package
}

func (e *Engine) GetEngine() *Engine {
	return e
}

func (e *Engine) MustModule(moduleName string) *pbsubstreams.Module {
	for _, module := range e.Package.Modules.Modules {
		if module.Name == moduleName {
			return module
		}
	}
	panic(fmt.Sprintf("MustModule %q not found", moduleName))
}

func (e *Engine) moduleOutputForName(moduleName string) (string, error) {
	//todo: call MustModule ...
	for _, module := range e.Package.Modules.Modules {
		if module.Name == moduleName {
			outputType := ""
			if storeModule := module.GetKindStore(); storeModule != nil {
				outputType = storeModule.ValueType
			}
			if mapModule := module.GetKindMap(); mapModule != nil {
				outputType = mapModule.OutputType
			}
			return outputType, nil
		}
	}
	return "", fmt.Errorf("MustModule %q not found", moduleName)
}

func (e *Engine) moduleOutput(module *pbsubstreams.Module) string {
	outputType := ""
	if storeModule := module.GetKindStore(); storeModule != nil {
		outputType = storeModule.ValueType
	}
	if mapModule := module.GetKindMap(); mapModule != nil {
		outputType = mapModule.OutputType
	}
	return outputType
}

func (e *Engine) FunctionSignature(module *pbsubstreams.Module) (*FunctionSignature, error) {
	switch module.Kind.(type) {
	case *pbsubstreams.Module_KindMap_:
		return e.mapFunctionSignature(module)
	case *pbsubstreams.Module_KindStore_:
		return e.storeFunctionSignature(module)
	default:
		return nil, fmt.Errorf("unknown MustModule kind: %T", module.Kind)
	}
}

func (e *Engine) Arguments(module *pbsubstreams.Module) ([]string, error) {
	var args []string
	return args, nil
}

func (e *Engine) mapFunctionSignature(module *pbsubstreams.Module) (*FunctionSignature, error) {
	mapModule := module.GetKindMap()

	inputs, err := e.ModuleArgument(module.Inputs)
	if err != nil {
		return nil, fmt.Errorf("generating MustModule intputs: %w", err)
	}

	outType := mapModule.OutputType
	if strings.HasPrefix(outType, "proto:") {
		outType = transformProtoType(outType)
	}

	fn := NewFunctionSignature(module.Name, "map", outType, pbsubstreams.Module_KindStore_UPDATE_POLICY_UNSET, inputs)

	return fn, nil
}

func (e *Engine) storeFunctionSignature(module *pbsubstreams.Module) (*FunctionSignature, error) {
	storeModule := module.GetKindStore()

	arguments, err := e.ModuleArgument(module.Inputs)
	if err != nil {
		return nil, fmt.Errorf("generating MustModule intputs: %w", err)
	}

	fn := NewFunctionSignature(module.Name, "store", "", storeModule.UpdatePolicy, arguments)

	return fn, nil
}

func (e *Engine) ModuleArgument(inputs []*pbsubstreams.Module_Input) (Arguments, error) {
	var out Arguments
	for _, input := range inputs {
		switch in := input.Input.(type) {
		case *pbsubstreams.Module_Input_Map_:
			inputType, err := e.moduleOutputForName(in.Map.ModuleName)
			if err != nil {
				return nil, fmt.Errorf("getting map type: %w", err)
			}
			if strings.HasPrefix(inputType, "proto:") {
				inputType = transformProtoType(inputType)
			}
			out = append(out, NewArgument(in.Map.ModuleName, inputType, input))
		case *pbsubstreams.Module_Input_Store_:
			mod := e.MustModule(in.Store.ModuleName)
			inputType := e.moduleOutput(mod)
			if strings.HasPrefix(inputType, "proto:") {
				inputType = transformProtoType(inputType)
			}

			out = append(out, NewArgument(in.Store.ModuleName, inputType, input))
		case *pbsubstreams.Module_Input_Source_:
			parts := strings.Split(in.Source.Type, ".")
			name := parts[len(parts)-1]
			name = strings.ToLower(name)

			switch in.Source.Type {
			case EthereumBlockManifest:
				out = append(out, NewArgument(name, EthereumBlockRust, input))
			case SubstreamsClock:
				out = append(out, NewArgument(name, SubstreamsClockRust, input))
			default:
				panic(fmt.Sprintf("unsupported source %q", in.Source.Type))
			}
		default:
			return nil, fmt.Errorf("unknown MustModule kind: %T", in)
		}
	}
	return out, nil
}

func IsDelta(input *pbsubstreams.Module_Input) bool {
	if storeInput := input.GetStore(); storeInput != nil {
		return storeInput.Mode == pbsubstreams.Module_Input_Store_DELTAS
	}
	return false
}

func IsStoreModule(module *pbsubstreams.Module) bool {
	switch module.Kind.(type) {
	case *pbsubstreams.Module_KindStore_:
		return true
	case *pbsubstreams.Module_KindMap_:
		return false
	default:
		return false
	}
}
func IsMapModule(module *pbsubstreams.Module) bool {
	switch module.Kind.(type) {
	case *pbsubstreams.Module_KindStore_:
		return false
	case *pbsubstreams.Module_KindMap_:
		return true
	default:
		return false
	}
}

func IsStoreInput(input *pbsubstreams.Module_Input) bool {
	switch input.Input.(type) {
	case *pbsubstreams.Module_Input_Store_:
		return true
	case *pbsubstreams.Module_Input_Map_:
		return false
	default:
		return false
	}
}
func IsMapInput(input *pbsubstreams.Module_Input) bool {
	switch input.Input.(type) {
	case *pbsubstreams.Module_Input_Store_:
		return false
	case *pbsubstreams.Module_Input_Map_:
		return true
	default:
		return false
	}
}

func ReadableStoreType(store *pbsubstreams.Module_KindStore, input *pbsubstreams.Module_Input_Store) string {
	t := store.ValueType

	if input.Mode == pbsubstreams.Module_Input_Store_DELTAS {
		if strings.HasPrefix(t, "proto") {
			t = transformProtoType(t)
			return fmt.Sprintf("substreams::store::Deltas<substreams::store::DeltaProto<%s>>", t)
		}
		t = StoreType[t]
		return fmt.Sprintf("substreams::store::Deltas<substreams::store::Delta%s>", t)

	}

	if strings.HasPrefix(t, "proto") {
		t = transformProtoType(t)
		return fmt.Sprintf("substreams::store::StoreGetProto<%s>", t)
	}
	t = StoreType[t]
	return fmt.Sprintf("substreams::store::StoreGet%s", t)
}
func WritableStoreType(store *pbsubstreams.Module_KindStore) string {
	t := store.ValueType

	p := pbsubstreams.Module_KindStore_UpdatePolicy_name[int32(store.UpdatePolicy)]
	p = UpdatePoliciesMap[p]
	if strings.HasPrefix(t, "proto") {
		t = transformProtoType(t)
		return fmt.Sprintf("substreams::store::Store%sProto<%s>", p, t)
	}
	t = StoreType[t]
	return fmt.Sprintf("substreams::store::Store%s%s", p, t)
}

func WritableStoreDeclaration(store *pbsubstreams.Module_KindStore) string {
	t := store.ValueType
	p := pbsubstreams.Module_KindStore_UpdatePolicy_name[int32(store.UpdatePolicy)]
	p = UpdatePoliciesMap[p]

	if strings.HasPrefix(t, "proto") {
		t = transformProtoType(t)
		return fmt.Sprintf("let store: substreams::store::Store%sProto<%s> = substreams::store::StoreSetProto::new();", p, t)
	}
	t = StoreType[t]
	return fmt.Sprintf("let store: substreams::store::Store%s%s = substreams::store::Store%s%s::new();", p, t, p, t)
}

func ReadableStoreDeclaration(name string, store *pbsubstreams.Module_KindStore, input *pbsubstreams.Module_Input_Store) string {
	t := store.ValueType
	isProto := strings.HasPrefix(t, "proto")
	if isProto {
		t = transformProtoType(t)
	}

	if input.Mode == pbsubstreams.Module_Input_Store_DELTAS {
		p := pbsubstreams.Module_KindStore_UpdatePolicy_name[int32(store.UpdatePolicy)]
		p = UpdatePoliciesMap[p]

		raw := fmt.Sprintf("let raw_%s_deltas = substreams::proto::decode_ptr::<substreams::pb::substreams::StoreDeltas>(%s_deltas_ptr, %s_deltas_len).unwrap().deltas;", name, name, name)
		delta := fmt.Sprintf("let %s_deltas: substreams::store::Deltas<substreams::store::Delta%s> = substreams::store::Deltas::new(raw_%s_deltas);", name, StoreType[t], name)
		if isProto {
			delta = fmt.Sprintf("let %s_deltas: substreams::store::Deltas<substreams::store::DeltaProto<%s>> = substreams::store::Deltas::new(raw_%s_deltas);", name, t, name)
		}
		return raw + "\n\t\t" + delta
	}

	if isProto {
		return fmt.Sprintf("let %s: substreams::store::StoreGetProto<%s>  = substreams::store::StoreGetProto::new(%s_ptr);", name, t, name)
	}

	t = StoreType[t]
	return fmt.Sprintf("let %s: substreams::store::StoreGet%s = substreams::store::StoreGet%s::new(%s_ptr);", name, t, t, name)

}

func transformProtoType(t string) string {
	t = strings.TrimPrefix(t, "proto:")
	parts := strings.Split(t, ".")
	if len(parts) >= 2 {
		t = strings.Join(parts[:len(parts)-1], "_")
	}
	return "pb::" + t + "::" + parts[len(parts)-1]
}

type FunctionSignature struct {
	Name        string
	Type        string
	OutputType  string
	StorePolicy string
	Arguments   Arguments
}

func NewFunctionSignature(name string, t string, outType string, storePolicy pbsubstreams.Module_KindStore_UpdatePolicy, arguments Arguments) *FunctionSignature {
	return &FunctionSignature{
		Name:        name,
		Type:        t,
		OutputType:  outType,
		StorePolicy: pbsubstreams.Module_KindStore_UpdatePolicy_name[int32(storePolicy)],
		Arguments:   arguments,
	}
}

type Arguments []*Argument

type Argument struct {
	Name        string
	Type        string
	ModuleInput *pbsubstreams.Module_Input
}

func NewArgument(name string, argType string, moduleInput *pbsubstreams.Module_Input) *Argument {
	return &Argument{
		Name:        name,
		Type:        argType,
		ModuleInput: moduleInput,
	}
}
