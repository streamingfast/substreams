package codegen

import (
	_ "embed"
	"fmt"
	"io"
	"strings"
	"text/template"

	"github.com/streamingfast/substreams/codegen/templates"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

//go:embed lib_ts.gotmpl
var libRsTemplate string

//go:embed externs.gotmpl
var externsTemplate string

const EthereumBlockManifest = "sf.ethereum.type.v2.Block"

const EthereumBlockRust = "substreams_ethereum::pb::sf_ethereum_type::v2::Block"

//UPDATE_POLICY_UNSET
//UPDATE_POLICY_SET
//UPDATE_POLICY_SET_IF_NOT_EXISTS
//UPDATE_POLICY_ADD
//UPDATE_POLICY_MIN
//UPDATE_POLICY_MAX
//UPDATE_POLICY_APPEND

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
	pkg *pbsubstreams.Package
	io.Writer
}

func NewGenerator(pkg *pbsubstreams.Package, writer io.Writer) *Generator {
	return &Generator{
		pkg:    pkg,
		Writer: writer,
	}
}

func (g *Generator) GenerateModRs() error {
	tmplGeneratedModRs, err := template.New("mod.rs").Parse(templates.ModRsTemplate)
	if err != nil {
		return fmt.Errorf("parsing mod.rs template: %w", err)
	}

	err = tmplGeneratedModRs.Execute(
		g.Writer,
		"generated",
	)

	if err != nil {
		return fmt.Errorf("executing mod.rs template: %w", err)
	}

	return nil
}

var utils = map[string]any{
	"contains":                 strings.Contains,
	"hasPrefix":                strings.HasPrefix,
	"hasSuffix":                strings.HasSuffix,
	"isDelta":                  IsDelta,
	"writableStoreDeclaration": WritableStoreDeclaration,
	"readableStoreDeclaration": ReadableStoreDeclaration,
}

func IsDelta(input *pbsubstreams.Module_Input) bool {
	if storeInput := input.GetStore(); storeInput != nil {
		return storeInput.Mode == pbsubstreams.Module_Input_Store_DELTAS
	}
	return false
}
func WritableStoreDeclaration(store pbsubstreams.Module_KindStore) string {
	t := store.ValueType
	p := pbsubstreams.Module_KindStore_UpdatePolicy_name[int32(store.UpdatePolicy)]
	p = UpdatePoliciesMap[p]

	if strings.HasPrefix(t, "proto") {
		t = strings.TrimPrefix(t, "proto:")
		t = strings.ReplaceAll(t, ".", "_")
		t = "crate::pb::" + t
		return fmt.Sprintf("let store: Store%sProto<%s> = StoreSetProto::new();", p, t)
	}
	return fmt.Sprintf("let store: Store%s%s = Store%s%s::new();", p, t, p, t)

}
func ReadableStoreDeclaration(name string, store pbsubstreams.Module_KindStore, input *pbsubstreams.Module_Input_Store) string {
	t := store.ValueType
	isProto := strings.HasPrefix(t, "proto")
	if isProto {
		t = strings.TrimPrefix(t, "proto:")
		t = strings.ReplaceAll(t, ".", "_")
		t = "crate::pb::" + t
	}

	if input.Mode == pbsubstreams.Module_Input_Store_DELTAS {
		p := pbsubstreams.Module_KindStore_UpdatePolicy_name[int32(store.UpdatePolicy)]
		p = UpdatePoliciesMap[p]

		if isProto {
			//return fmt.Sprintf("let raw_totals_deltas = substreams::proto::decode_ptr::<substreams::pb::substreams::StoreDeltas>(totals_deltas_ptr, totals_deltas_len).unwrap().deltas;")
			//return fmt.Sprintf("let %s_deltas: store::Deltas<Delta%s> = substreams::store::Deltas::new(%s_deltas", name, p, name)
		}
		raw := fmt.Sprintf("let raw_%s_deltas = substreams::proto::decode_ptr::<substreams::pb::substreams::StoreDeltas>(totals_deltas_ptr, totals_deltas_len).unwrap().deltas;", name)
		delta := fmt.Sprintf("\t\tlet %s_deltas: store::Deltas<Delta%s> = substreams::store::Deltas::new(raw_%s_deltas);", name, p, name)
		return raw + "\n" + delta
	}

	if strings.HasPrefix(t, "proto") {
		return fmt.Sprintf("let %s: StoreGetProto<%s> = StoreGetProto::new();", name, t)
	}

	return fmt.Sprintf("let %s: StoreGet%s = StoreGet%s::new();", name, t, t)

}

func (g *Generator) Generate() (err error) {
	//err := g.GenerateLib()
	//if err != nil {
	//	return fmt.Errorf("generating lib.ts: %w", err)
	//}
	err = g.GenerateExterns()
	if err != nil {
		return fmt.Errorf("generating externs.ts: %w", err)
	}

	return nil
}

func (g *Generator) GenerateExterns() error {
	tmpl, err := template.New("externs").Funcs(utils).Parse(externsTemplate)
	if err != nil {
		return fmt.Errorf("parsing externs template: %w", err)
	}

	err = tmpl.Execute(
		g.Writer,
		&Engine{Package: g.pkg},
	)
	if err != nil {
		return fmt.Errorf("executing externs template: %w", err)
	}

	return nil
}

func (g *Generator) GenerateLib() error {
	//todo:
	// > Use fully qualified fields
	//  1. Hard-code the block type (for ethereum in the first place)
	//	2. Fully qualified path for the prototypes defined in the yaml

	tmplGeneratedRs, err := template.New("generated.rs").Funcs(utils).Parse(templates.LibRsTemplate)
	if err != nil {
		return fmt.Errorf("parsing generated.rs template: %w", err)
	}

	err = tmplGeneratedRs.Execute(
		g.Writer,
		&Engine{Package: g.pkg},
	)

	if err != nil {
		return fmt.Errorf("executing generated.rs template: %w", err)
	}

	return nil
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

	outType := strings.TrimPrefix(mapModule.OutputType, "proto:")
	outType = strings.ReplaceAll(outType, ".", ":")
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

type Arguments map[string]*Argument

func (e *Engine) ModuleArgument(inputs []*pbsubstreams.Module_Input) (Arguments, error) {
	// fixme: refactor this
	out := make(Arguments)
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
			out[in.Map.ModuleName] = NewArgument(in.Map.ModuleName, inputType, input)
		case *pbsubstreams.Module_Input_Store_:
			mod := e.MustModule(in.Store.ModuleName)
			inputType := e.moduleOutput(mod)
			if strings.HasPrefix(inputType, "proto:") {
				inputType = transformProtoType(inputType)
			}

			out[in.Store.ModuleName] = NewArgument(in.Store.ModuleName, inputType, input)
		case *pbsubstreams.Module_Input_Source_:
			parts := strings.Split(in.Source.Type, ".")
			name := parts[len(parts)-1]
			name = strings.ToLower(name)

			switch in.Source.Type {
			case EthereumBlockManifest:
				out[name] = NewArgument(name, EthereumBlockRust, input)
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

func transformProtoType(protoType string) string {
	outputType := strings.TrimPrefix(protoType, "proto:")
	elements := strings.Split(outputType, ".")
	return fmt.Sprintf("crate::pb::%s::%s", elements[0], elements[len(elements)-1])
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
