package pbsubstreams

import (
	"fmt"
	"strings"
)

type ModuleKind int

const (
	ModuleKindStore = ModuleKind(iota)
	ModuleKindMap
	ModuleKindBlockIndex
)

func (m *Module) BlockFilterQueryString() (string, error) {
	if m.BlockFilter == nil {
		return "", nil
	}
	switch q := m.BlockFilter.Query.(type) {
	case *Module_BlockFilter_QueryString:
		return q.QueryString, nil
	case *Module_BlockFilter_QueryFromParams:
		for _, input := range m.Inputs {
			if p := input.GetParams(); p != nil {
				return p.Value, nil
			}
		}
		return "", fmt.Errorf("getting blockFilterQueryString: no params input")
	default:
		return "", fmt.Errorf("getting blockFilterQueryString: unsupported query type")
	}
}

func (x *Module) ModuleKind() ModuleKind {
	switch x.Kind.(type) {
	case *Module_KindMap_:
		return ModuleKindMap
	case *Module_KindStore_:
		return ModuleKindStore
	case *Module_KindBlockIndex_:
		return ModuleKindBlockIndex
	}
	panic("unsupported kind")
}

func (x *Module_Input) Pretty() string {
	var result string
	switch x.Input.(type) {
	case *Module_Input_Map_:
		result = x.GetMap().GetModuleName()
	case *Module_Input_Store_:
		result = x.GetStore().GetModuleName()
	case *Module_Input_Source_:
		result = x.GetSource().GetType()
	case *Module_Input_Params_:
		result = x.GetParams().GetValue()
	default:
		result = "unknown"
	}

	return strings.TrimSpace(result)
}

func (x Module_KindStore_UpdatePolicy) Pretty() string {
	return strings.TrimPrefix(strings.ToLower(x.String()), "update_policy_")
}

func (x Module_Input_Store_Mode) Pretty() string {
	return strings.ToLower(Module_Input_Store_Mode_name[int32(x)])
}
