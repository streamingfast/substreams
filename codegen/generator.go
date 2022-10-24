package codegen

import (
	"fmt"
	"os"
	"strings"
	"text/template"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

type Generator struct {
	pkg *pbsubstreams.Package
}

func NewGenerator(pkg *pbsubstreams.Package) *Generator {
	return &Generator{
		pkg: pkg,
	}
}

func (g *Generator) Generate() error {
	tmpl, err := template.New("lib.rs").Parse(libRsTemplate)
	if err != nil {
		return fmt.Errorf("parsing lib rs template: %w", err)
	}

	err = tmpl.Execute(
		os.Stdout,
		&Engine{Package: g.pkg},
	)
	if err != nil {
		return fmt.Errorf("executing lib.rs template: %w", err)
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
			return strings.TrimPrefix(outputType, "proto:"), nil
		}
	}
	return "", fmt.Errorf("module %q not found", moduleName)
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

func (e *Engine) mapFunctionSignature(module *pbsubstreams.Module) (*FunctionSignature, error) {
	mapModule := module.GetKindMap()

	inputs, err := e.ModuleInputs(module.Inputs)
	if err != nil {
		return nil, fmt.Errorf("generating module intputs: %w", err)
	}

	fn := &FunctionSignature{
		Name:        module.Name,
		Type:        "map",
		InputParams: inputs,
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

	fn := &FunctionSignature{
		Name:        module.Name,
		Type:        "store",
		InputParams: inputs,
		OutputType:  strings.TrimPrefix(storeModule.ValueType, "proto:"),
	}

	return fn, nil
}

func (e *Engine) ModuleInputs(inputs []*pbsubstreams.Module_Input) (map[string]string, error) {
	out := make(map[string]string)
	for _, input := range inputs {
		switch in := input.Input.(type) {
		case *pbsubstreams.Module_Input_Map_:
			inputType, err := e.moduleOutput(in.Map.ModuleName)
			if err != nil {
				return nil, fmt.Errorf("getting map type: %w", err)
			}
			out[in.Map.ModuleName] = inputType
		case *pbsubstreams.Module_Input_Store_:
			inputType, err := e.moduleOutput(in.Store.ModuleName)
			if err != nil {
				return nil, fmt.Errorf("getting store type: %w", err)
			}
			out[in.Store.ModuleName] = inputType
		case *pbsubstreams.Module_Input_Source_:
			parts := strings.Split(in.Source.Type, ".")
			name := parts[len(parts)-1]
			name = strings.ToLower(name)
			out[name] = in.Source.Type
		default:
			return nil, fmt.Errorf("unknown module kind: %T", in)
		}
	}
	return out, nil
}

type FunctionSignature struct {
	Name        string
	Type        string
	OutputType  string
	InputParams map[string]string
}
