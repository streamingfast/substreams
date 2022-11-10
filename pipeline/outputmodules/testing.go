package outputmodules

func TestNew() *Graph {
	return &Graph{outputModuleMap: make(map[string]bool)}
}
