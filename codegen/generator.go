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

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

//go:embed templates/lib.gotmpl
var libRsTemplate string

//go:embed templates/externs.gotmpl
var externsTemplate string

//go:embed templates/substreams.gotmpl
var substreamsTemplate string

//go:embed templates/mod.gotmpl
var modTemplate string

const EthereumBlockManifest = "sf.ethereum.type.v2.Block"
const EthereumBlockRust = "substreams_ethereum::pb::eth::v2::Block"

var StoreType = map[string]string{
	"bytes":      "Raw",
	"string":     "String",
	"bigint":     "BigInt",
	"bigdecimal": "BigDecimal",
	"bigfloat":   "BigDecimal",
	"int64":      "Int64",
	"i64":        "Int64",
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
	"float64":                         "Float64",
}

type Generator struct {
	pkg      *pbsubstreams.Package
	basePath string
}

func NewGenerator(pkg *pbsubstreams.Package, basePath string) *Generator {
	return &Generator{
		pkg:      pkg,
		basePath: basePath,
	}
}

func (g *Generator) Generate() (err error) {
	var writer io.Writer

	if err := os.MkdirAll(g.basePath, os.ModePerm); err != nil {
		return fmt.Errorf("creating src directory %v: %w", g.basePath, err)
	}

	generatedFolder := filepath.Join(g.basePath, "generated")
	if err := os.MkdirAll(generatedFolder, os.ModePerm); err != nil {
		return fmt.Errorf("creating generated directory %v: %w", g.basePath, err)
	}

	libFilePath := filepath.Join(g.basePath, "lib.rs")
	if _, err := os.Stat(libFilePath); errors.Is(err, os.ErrNotExist) {
		// path/to/whatever does not exist
		f, err := os.Create(libFilePath)
		if err != nil {
			return fmt.Errorf("creating file lib.rs in %q: %w", g.basePath, err)
		}
		writer = f

		err = g.GenerateLib(writer)
		if err != nil {
			return fmt.Errorf("generating lib.ts: %w", err)
		}
	}

	f, err := os.Create(filepath.Join(generatedFolder, "externs.rs"))
	if err != nil {
		return fmt.Errorf("creating file externs.rs in %q: %w", generatedFolder, err)
	}
	writer = f

	err = g.GenerateExterns(writer)
	if err != nil {
		return fmt.Errorf("generating externs.ts: %w", err)
	}

	f, err = os.Create(filepath.Join(generatedFolder, "substreams.rs"))
	if err != nil {
		return fmt.Errorf("creating file substreams.rs in %q: %w", generatedFolder, err)
	}
	writer = f
	err = g.GenerateSubstreams(writer)
	if err != nil {
		return fmt.Errorf("generating substreams.rs: %w", err)
	}

	f, err = os.Create(filepath.Join(generatedFolder, "mod.rs"))
	if err != nil {
		return fmt.Errorf("creating file mod.rs in %q: %w", generatedFolder, err)
	}
	writer = f
	err = g.GenerateMod(writer)
	if err != nil {
		return fmt.Errorf("generating mod.rss: %w", err)
	}

	return nil
}

func (g *Generator) GenerateExterns(writer io.Writer) error {
	engine := &Engine{Package: g.pkg}
	utils["getEngine"] = engine.GetEngine

	tmpl, err := template.New("externs").Funcs(utils).Parse(externsTemplate)
	if err != nil {
		return fmt.Errorf("parsing externs template: %w", err)
	}

	err = tmpl.Execute(
		writer,
		engine,
	)
	if err != nil {
		return fmt.Errorf("executing externs template: %w", err)
	}

	return nil
}

func (g *Generator) GenerateLib(writer io.Writer) error {
	engine := &Engine{Package: g.pkg}
	utils["getEngine"] = engine.GetEngine
	tmplGeneratedRs, err := template.New("lib").Funcs(utils).Parse(libRsTemplate)
	if err != nil {
		return fmt.Errorf("parsing lib template: %w", err)
	}

	err = tmplGeneratedRs.Execute(
		writer,
		engine,
	)

	if err != nil {
		return fmt.Errorf("executing lib template: %w", err)
	}

	return nil
}

func (g *Generator) GenerateSubstreams(writer io.Writer) error {
	engine := &Engine{Package: g.pkg}
	utils["getEngine"] = engine.GetEngine

	tmpl, err := template.New("substreams").Funcs(utils).Parse(substreamsTemplate)
	if err != nil {
		return fmt.Errorf("parsing substreams template: %w", err)
	}

	err = tmpl.Execute(
		writer,
		engine,
	)

	if err != nil {
		return fmt.Errorf("executing substreams template: %w", err)
	}

	return nil
}

func (g *Generator) GenerateMod(writer io.Writer) error {
	engine := &Engine{Package: g.pkg}
	utils["getEngine"] = engine.GetEngine

	tmpl, err := template.New("mod").Funcs(utils).Parse(modTemplate)
	if err != nil {
		return fmt.Errorf("parsing mod template: %w", err)
	}

	err = tmpl.Execute(
		writer,
		engine,
	)

	if err != nil {
		return fmt.Errorf("executing mod template: %w", err)
	}

	return nil
}

var utils = map[string]any{
	"contains":                 strings.Contains,
	"hasPrefix":                strings.HasPrefix,
	"hasSuffix":                strings.HasSuffix,
	"isDelta":                  IsDelta,
	"writableStoreDeclaration": WritableStoreDeclaration,
	"writableStoreType":        WritableStoreType,
	"readableStoreDeclaration": ReadableStoreDeclaration,
	"readableStoreType":        ReadableStoreType,
}

func (e *Engine) GetEngine() *Engine {
	return e
}

func IsDelta(input *pbsubstreams.Module_Input) bool {
	if storeInput := input.GetStore(); storeInput != nil {
		return storeInput.Mode == pbsubstreams.Module_Input_Store_DELTAS
	}
	return false
}

func ReadableStoreType(store pbsubstreams.Module_KindStore, input *pbsubstreams.Module_Input_Store) string {
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
func WritableStoreType(store pbsubstreams.Module_KindStore) string {
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

func WritableStoreDeclaration(store pbsubstreams.Module_KindStore) string {
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

func ReadableStoreDeclaration(name string, store pbsubstreams.Module_KindStore, input *pbsubstreams.Module_Input_Store) string {
	t := store.ValueType
	isProto := strings.HasPrefix(t, "proto")
	if isProto {
		t = transformProtoType(t)
	}

	if input.Mode == pbsubstreams.Module_Input_Store_DELTAS {
		p := pbsubstreams.Module_KindStore_UpdatePolicy_name[int32(store.UpdatePolicy)]
		p = UpdatePoliciesMap[p]

		raw := fmt.Sprintf("let raw_%s_deltas = substreams::proto::decode_ptr::<substreams::pb::substreams::StoreDeltas>(%s_deltas_ptr, %s_deltas_len).unwrap().deltas;", name, name, name)
		delta := fmt.Sprintf("\t\tlet %s_deltas: substreams::store::Deltas<substreams::store::Delta%s> = substreams::store::Deltas::new(raw_%s_deltas);", name, StoreType[t], name)
		if isProto {
			delta = fmt.Sprintf("\t\tlet %s_deltas: substreams::store::Deltas<substreams::store::DeltaProto<%s>> = substreams::store::Deltas::new(raw_%s_deltas);", name, t, name)
		}
		return raw + "\n" + delta
	}

	if isProto {
		return fmt.Sprintf("let %s: substreams::store::StoreGetProto<%s>  = substreams::store::StoreGetProto::new(%s_ptr);", name, t, name)
	}

	t = StoreType[t]
	return fmt.Sprintf("let %s: substreams::store::StoreGet%s = substreams::store::StoreGet%s::new(%s_ptr);", name, t, t, name)

}

type Engine struct {
	Package *pbsubstreams.Package
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
	args = append(args, "salyut")
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
			default:
				panic(fmt.Sprintf("unsupported source %q", in.Source.Type))
			}
		default:
			return nil, fmt.Errorf("unknown MustModule kind: %T", in)
		}
	}
	return out, nil
}

func transformOutputType(moduleValueType string) string {
	outputType := strings.TrimPrefix(moduleValueType, "proto:")
	elements := strings.Split(outputType, ".")
	return fmt.Sprintf("crate::pb::%s::%s", elements[0], elements[len(elements)-1])
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
