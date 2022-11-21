package pbsubstreams

type ModuleKind int

const (
	ModuleKindStore = ModuleKind(iota)
	ModuleKindMap
)

func (x *Module) ModuleKind() ModuleKind {
	switch x.Kind.(type) {
	case *Module_KindMap_:
		return ModuleKindMap
	case *Module_KindStore_:
		return ModuleKindStore
	}
	panic("unsupported kind")
}
