package codegen

import (
	"fmt"
	"io"
	"strings"
	"text/template"

	"github.com/streamingfast/substreams/codegen/templates"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

const EthereumBlockManifest = "sf.ethereum.type.v2.Block"
const EthereumBlockRust = "substreams_ethereum::pb::eth::v2::Block"

var WritableStore = [21]string{
	"StoreSetRaw",
	"StoreSetBigInt",
	"StoreSetBigDecimal",
	"StoreSetProto",
	"StoreSetI64",
	"StoreSetFloat64",
	"StoreSetIfNotExistsRaw",
	"StoreSetIfNotExistsProto",
	"StoreAddInt64",
	"StoreAddFloat64",
	"StoreAddBigDecimal",
	"StoreAddBigInt",
	"StoreMaxInt64",
	"StoreMaxBigInt",
	"StoreMaxFloat64",
	"StoreMaxBigDecimal",
	"StoreMinInt64",
	"StoreMinBigInt",
	"StoreMinFloat64",
	"StoreMinBigDecimal",
	"StoreAppend",
}

var ReadableStore = [6]string{
	"StoreGetI64",
	"StoreGetFloat64",
	"StoreGetBigDecimal",
	"StoreGetBigInt",
	"StoreGetProto",
	"StoreGetRaw",
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

func (g *Generator) GenerateGeneratedRs() error {
	//todo:
	// > Use fully qualified fields
	//  1. Hard-code the block type (for ethereum in the first place)
	//	2. Fully qualified path for the prototypes defined in the yaml

	tmplGeneratedRs, err := template.New("generated.rs").Parse(templates.LibRsTemplate)
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

func (e *Engine) moduleOutput(moduleName string) (string, error) {
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
	return "", fmt.Errorf("module %q not found", moduleName)
}

func (e *Engine) ExternFunctionSignature(module *pbsubstreams.Module) (*FunctionSignature, error) {
	switch module.Kind.(type) {
	case *pbsubstreams.Module_KindMap_:
		return e.mapExternFunctionSignature(module)
	case *pbsubstreams.Module_KindStore_:
		return e.storeExternFunctionSignature(module)
	default:
		return nil, fmt.Errorf("unknown module kind: %T", module.Kind)
	}
}

func (e *Engine) FunctionSignature(module *pbsubstreams.Module) (*FunctionSignature, error) {
	switch module.Kind.(type) {
	case *pbsubstreams.Module_KindMap_:
		return e.mapFunctionSignature(module)
	case *pbsubstreams.Module_KindStore_:
		return e.storeFunctionSignature(module)
	default:
		return nil, fmt.Errorf("unknown module kind: %T", module.Kind)
	}
}

func (e *Engine) Arguments(module *pbsubstreams.Module) ([]string, error) {
	var args []string
	args = append(args, "salyut")
	return args, nil
}

func (e *Engine) mapExternFunctionSignature(module *pbsubstreams.Module) (*FunctionSignature, error) {
	inputs, err := e.ModuleInputs(module.Inputs)
	if err != nil {
		return nil, fmt.Errorf("generating module intputs: %w", err)
	}

	transformedInputParams := make(map[string][]*InputParam)

	for varName, input := range inputs {
		if isWritableStore(input) {
			return nil, fmt.Errorf("mapper substreams can't write in a store, check substreams 'kind'")
		} else if isReadableStore(input) {
			transformedInputParams[varName] = []*InputParam{
				{
					Name: fmt.Sprintf("%s_idx", varName),
					Type: "u32",
				},
			}
		} else {
			transformedInputParams[varName] = []*InputParam{
				{
					Name: fmt.Sprintf("%s_ptr", varName),
					Type: "*mut u8",
				},
				{
					Name: fmt.Sprintf("%s_len", varName),
					Type: "usize",
				},
			}
		}
	}

	fn := &FunctionSignature{
		Name:        module.Name,
		Type:        "map",
		InputParams: transformedInputParams,
		OutputType:  transformOutputType(module.Output.Type),
	}

	return fn, nil
}

func isWritableStore(store string) bool {
	for _, item := range WritableStore {
		if item == store {
			return true
		}
	}
	return false
}

func isReadableStore(store string) bool {
	for _, item := range ReadableStore {
		if item == store {
			return true
		}
	}
	return false
}

func (e *Engine) storeExternFunctionSignature(module *pbsubstreams.Module) (*FunctionSignature, error) {
	storeModule := module.GetKindStore()

	inputs, err := e.ModuleInputs(module.Inputs)
	if err != nil {
		return nil, fmt.Errorf("generating module intputs: %w", err)
	}

	transformedInputParams := make(map[string][]*InputParam)

	for varName, input := range inputs {
		if isWritableStore(input) {
			return nil, fmt.Errorf("store substreams can't read and write at the same time")
		} else if isReadableStore(input) {
			transformedInputParams[varName] = []*InputParam{
				{
					Name: fmt.Sprintf("%s_idx", varName),
					Type: "u32",
				},
			}
		} else {
			transformedInputParams[varName] = []*InputParam{
				{
					Name: fmt.Sprintf("%s_ptr", varName),
					Type: "*mut u8",
				},
				{
					Name: fmt.Sprintf("%s_len", varName),
					Type: "usize",
				},
			}
		}
	}

	fn := &FunctionSignature{
		Name:        module.Name,
		Type:        "store",
		InputParams: transformedInputParams,
		OutputType:  transformOutputType(storeModule.ValueType),
	}

	return fn, nil
}

func (e *Engine) mapFunctionSignature(module *pbsubstreams.Module) (*FunctionSignature, error) {
	mapModule := module.GetKindMap()

	inputs, err := e.ModuleInputs(module.Inputs)
	if err != nil {
		return nil, fmt.Errorf("generating module intputs: %w", err)
	}

	transformedInputParams := make(map[string][]*InputParam)

	for varName, input := range inputs {
		if isWritableStore(input) {
			return nil, fmt.Errorf("store substreams can't read and write at the same time")
		} else if isReadableStore(input) {
			// todo
		} else {
			transformedInputParams[varName] = []*InputParam{
				{
					Name: varName,
					Type: input,
				},
			}
		}
	}

	fn := &FunctionSignature{
		Name:        module.Name,
		Type:        "map",
		InputParams: transformedInputParams,
		OutputType:  strings.TrimPrefix(mapModule.OutputType, "proto:"),
	}

	return fn, nil
}

func (e *Engine) storeFunctionSignature(module *pbsubstreams.Module) (*FunctionSignature, error) {
	storeModule := module.GetKindStore()

	inputs, err := e.ModuleInputs(module.Inputs)
	if err != nil {
		return nil, fmt.Errorf("generating module intputs: %w", err)
	}

	// if isProto
	// 	trim proto and create StoreSetProto<crate .... >
	// else
	// 	StoreSet{Type}

	if strings.HasPrefix(storeModule.ValueType, "proto:") {
		inputs["store"] = fmt.Sprintf("StoreSetProto<%s>", transformProtoType(storeModule.ValueType))
	} else {
		inputs["store"] = fmt.Sprintf("StoreSetProto")
	}

	fn := &FunctionSignature{
		Name:        module.Name,
		Type:        "store",
		InputParams: nil,
	}

	return fn, nil
}

func (e *Engine) ModuleInputs(inputs []*pbsubstreams.Module_Input) (map[string]string, error) {
	// fixme: refactor this
	out := make(map[string]string)
	for _, input := range inputs {
		switch in := input.Input.(type) {
		case *pbsubstreams.Module_Input_Map_:
			inputType, err := e.moduleOutput(in.Map.ModuleName)
			if err != nil {
				return nil, fmt.Errorf("getting map type: %w", err)
			}
			if strings.HasPrefix(inputType, "proto:") {
				inputType = transformProtoType(inputType)
			}
			out[in.Map.ModuleName] = inputType
		case *pbsubstreams.Module_Input_Store_:
			inputType, err := e.moduleOutput(in.Store.ModuleName)
			if err != nil {
				return nil, fmt.Errorf("getting store type: %w", err)
			}
			if strings.HasPrefix(inputType, "proto:") {
				inputType = transformProtoType(inputType)
			}
			out[in.Store.ModuleName] = inputType
		case *pbsubstreams.Module_Input_Source_:
			parts := strings.Split(in.Source.Type, ".")
			name := parts[len(parts)-1]
			name = strings.ToLower(name)

			switch in.Source.Type {
			case EthereumBlockManifest:
				out[name] = EthereumBlockRust
			default:
				out[name] = in.Source.Type
			}
		default:
			return nil, fmt.Errorf("unknown module kind: %T", in)
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
	InputParams map[string][]*InputParam
}

type InputParam struct {
	Name string
	Type string
}
