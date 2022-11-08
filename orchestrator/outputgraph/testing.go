package outputgraph

func TestNew() *OutputModulesGraph {
	return &OutputModulesGraph{outputModuleMap: make(map[string]bool)}
}
