package manifest

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/streamingfast/bstream"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/yourbasic/graph"
	"go.uber.org/zap"
)

type ModuleGraph struct {
	*graph.Mutable

	currentHashesCache map[string][]byte // moduleName => hash

	modules         []*pbsubstreams.Module
	moduleIndex     map[string]int
	indexIndex      map[int]*pbsubstreams.Module
	inputOrderIndex map[string]map[string]int
}

func NewModuleGraph(modules []*pbsubstreams.Module) (*ModuleGraph, error) {
	g := &ModuleGraph{
		Mutable:            graph.New(len(modules)),
		modules:            modules,
		moduleIndex:        make(map[string]int),
		indexIndex:         make(map[int]*pbsubstreams.Module),
		currentHashesCache: make(map[string][]byte),
		inputOrderIndex:    map[string]map[string]int{},
	}

	for i, module := range modules {
		g.moduleIndex[module.Name] = i
		g.indexIndex[i] = module
		g.inputOrderIndex[module.Name] = map[string]int{}
	}

	for i, module := range modules {
		for j, input := range module.Inputs {
			var moduleName string
			if v := input.GetMap(); v != nil {
				moduleName = v.GetModuleName()
			} else if v := input.GetStore(); v != nil {
				moduleName = v.GetModuleName()
			} else if v := input.GetSource(); v != nil {
				moduleName = v.GetType()
			} else if v := input.GetParams(); v != nil {
				moduleName = v.GetValue()
			}

			if moduleName == "" {
				continue
			}

			if j, found := g.moduleIndex[moduleName]; found {
				g.AddCost(i, j, 1)
			}

			g.inputOrderIndex[module.Name][moduleName] = j
		}
		if module.BlockFilter != nil {
			moduleName := module.BlockFilter.Module
			if j, found := g.moduleIndex[moduleName]; found {
				g.AddCost(i, j, 1)
				g.inputOrderIndex[module.Name][moduleName] = j
			}
		}
	}

	if !graph.Acyclic(g) {
		return nil, fmt.Errorf("modules graph has a cycle")
	}

	return g, nil
}

// ResetGraphHashes is to be called when you want to force a recomputation of the module hashes.
func (graph *ModuleGraph) ResetGraphHashes() {
	graph.currentHashesCache = make(map[string][]byte)
	// TODO: when we support multiple `initialBlock` for a given `moduleName`, we'll want
	// to make sure we call this between the boundaries, to reset the module hashes.
}

func (g *ModuleGraph) GetSources() []string {
	var sources []string
	for _, module := range g.modules {
		for _, input := range module.Inputs {
			if s := input.GetSource(); s != nil {
				sources = append(sources, s.GetType())
			}
		}
	}
	return sources
}

func computeInitialBlock(modules []*pbsubstreams.Module, g *ModuleGraph) error {
	for _, module := range modules {
		if module.InitialBlock == UNSET {
			moduleIndex := g.moduleIndex[module.Name]
			startBlock, err := startBlockForModule(moduleIndex, g)
			if err != nil {
				return err
			}

			module.InitialBlock = startBlock
			zlog.Info("computed start block", zap.String("module_name", module.Name), zap.Uint64("start_block", startBlock))
		}
	}
	return nil
}

func startBlockForModule(moduleIndex int, g *ModuleGraph) (out uint64, err error) {
	parentsInitialBlock := int64(-1)
	g.Visit(moduleIndex, func(w int, c int64) bool {
		parent := g.modules[w]
		currentInitialBlock := int64(-1)
		if parent.InitialBlock == UNSET {
			var newVal uint64
			newVal, err = startBlockForModule(w, g)
			if err != nil {
				return true
			}
			currentInitialBlock = int64(newVal)
		} else {
			currentInitialBlock = int64(parent.GetInitialBlock())
		}

		if parentsInitialBlock == -1 {
			if currentInitialBlock != -1 {
				parentsInitialBlock = currentInitialBlock
			}
			return false
		}
		if parentsInitialBlock != currentInitialBlock {
			err = fmt.Errorf("cannot deterministically determine the initialBlock for module %q; multiple inputs have conflicting initial blocks defined or inherited", g.modules[moduleIndex].Name)
			return true
		}
		return false
	})
	if err != nil {
		return uint64(0), err
	}

	if parentsInitialBlock == -1 {
		return bstream.GetProtocolFirstStreamableBlock, nil
	}
	return uint64(parentsInitialBlock), nil
}

func (g *ModuleGraph) ModuleNameFromIndex(index int) string {
	return g.indexIndex[index].Name
}

func (g *ModuleGraph) ModuleIndexFromName(name string) (int, bool) {
	v, ok := g.moduleIndex[name]
	return v, ok
}

func (g *ModuleGraph) Modules() []string {
	var modules []string
	for _, module := range g.modules {
		modules = append(modules, module.Name)
	}

	SortModuleNamesByGraphTopology(modules, g)

	return modules
}

func (g *ModuleGraph) MapModules() []string {
	var modules []string
	for _, module := range g.modules {
		if _, ok := module.Kind.(*pbsubstreams.Module_KindMap_); ok {
			modules = append(modules, module.Name)
		}
	}

	SortModuleNamesByGraphTopology(modules, g)

	return modules
}

func (g *ModuleGraph) TopologicalSort() ([]*pbsubstreams.Module, bool) {
	order, ok := graph.TopSort(g)
	if !ok {
		return nil, ok
	}

	var res []*pbsubstreams.Module
	for _, i := range order {
		res = append(res, g.indexIndex[i])
	}

	return res, ok
}

func (g *ModuleGraph) TopologicalSortKnownModules(known map[string]bool) ([]*pbsubstreams.Module, bool) {
	order, ok := graph.TopSort(g)
	if !ok {
		return nil, ok
	}

	var res []*pbsubstreams.Module
	for _, i := range order {
		if known[g.indexIndex[i].Name] {
			res = append(res, g.indexIndex[i])
		}
	}

	return res, ok
}

func (g *ModuleGraph) AncestorsOf(moduleName string) ([]*pbsubstreams.Module, error) {
	if _, found := g.moduleIndex[moduleName]; !found {
		return nil, fmt.Errorf("could not find module %s in graph", moduleName)
	}

	_, distances := graph.ShortestPaths(g, g.moduleIndex[moduleName])

	var res []*pbsubstreams.Module
	for i, d := range distances {
		if d >= 1 {
			res = append(res, g.indexIndex[i])
		}
	}

	return res, nil
}

func (g *ModuleGraph) AncestorStoresOf(moduleName string) ([]*pbsubstreams.Module, error) {
	ancestors, err := g.AncestorsOf(moduleName)
	if err != nil {
		return nil, err
	}

	result := make([]*pbsubstreams.Module, 0, len(ancestors))
	for _, a := range ancestors {
		kind := a.GetKindStore()
		if kind != nil {
			result = append(result, a)
		}
	}

	return result, nil
}

func (g *ModuleGraph) Context(moduleName string) (parents []string, children []string, err error) {
	// loop over inputs to get parents
	mod, found := g.ModuleIndexFromName(moduleName)
	if !found {
		return nil, nil, fmt.Errorf("could not find module %s in graph", moduleName)
	}

	inputSeen := map[string]bool{}
	for _, input := range g.modules[mod].Inputs {
		if inputSeen[input.Pretty()] {
			continue
		}
		parents = append(parents, input.Pretty())
		inputSeen[input.Pretty()] = true
	}

	for _, m := range g.MustChildrenOf(moduleName) {
		children = append(children, m.Name)
	}

	sort.Strings(children)

	return
}

func (g *ModuleGraph) MustParentsOf(moduleName string) []*pbsubstreams.Module {
	res, err := g.ParentsOf(moduleName)
	if err != nil {
		panic(err)
	}
	return res
}

func (g *ModuleGraph) ParentsOf(moduleName string) ([]*pbsubstreams.Module, error) {
	if _, found := g.moduleIndex[moduleName]; !found {
		return nil, fmt.Errorf("could not find module %s in graph", moduleName)
	}

	_, distances := graph.ShortestPaths(g, g.moduleIndex[moduleName])

	var res []*pbsubstreams.Module
	for i, d := range distances {
		if d == 1 {
			res = append(res, g.indexIndex[i])
		}
	}

	sort.Slice(res, func(i, j int) bool {
		return g.inputOrderIndex[moduleName][res[i].Name] < g.inputOrderIndex[moduleName][res[j].Name]
	})

	return res, nil
}

func (g *ModuleGraph) MustChildrenOf(moduleName string) []*pbsubstreams.Module {
	res, err := g.ChildrenOf(moduleName)
	if err != nil {
		panic(err)
	}
	return res
}

func (g *ModuleGraph) ChildrenOf(moduleName string) ([]*pbsubstreams.Module, error) {
	if _, found := g.moduleIndex[moduleName]; !found {
		return nil, fmt.Errorf("could not find module %s in graph", moduleName)
	}

	var res []*pbsubstreams.Module
	resSet := map[string]*pbsubstreams.Module{}
	for _, module := range g.modules {
		_, distances := graph.ShortestPaths(g, g.moduleIndex[module.Name])
		for i, d := range distances {
			if d == 1 {
				if g.indexIndex[i].Name == moduleName {
					resSet[module.Name] = module
				}
			}
		}
	}

	for _, module := range resSet {
		res = append(res, module)
	}

	sortedModules, ok := g.TopologicalSort()
	if !ok {
		return nil, fmt.Errorf("could not get topological sort of module graph")
	}

	topologicalIndex := map[string]int{}

	for i, node := range sortedModules {
		topologicalIndex[node.Name] = i
	}

	sort.Slice(res, func(i, j int) bool {
		return topologicalIndex[res[i].Name] > topologicalIndex[res[j].Name]
	})

	return res, nil
}

func (g *ModuleGraph) HasStatefulDependencies(moduleName string) (bool, error) {
	stores, err := g.StoresDownTo(moduleName)
	if err != nil {
		return false, fmt.Errorf("getting stores down to %s: %w", moduleName, err)
	}

	if len(stores) > 0 {
		return true, nil
	}

	return false, nil
}

func (g *ModuleGraph) StoresDownTo(moduleName string) ([]*pbsubstreams.Module, error) {
	alreadyAdded := map[string]bool{}
	topologicalIndex := map[string]int{}

	sortedModules, ok := g.TopologicalSort()
	if !ok {
		return nil, fmt.Errorf("could not get topological sort of module graph")
	}

	for i, node := range sortedModules {
		topologicalIndex[node.Name] = i
	}

	var res []*pbsubstreams.Module
	if _, found := g.moduleIndex[moduleName]; !found {
		return nil, fmt.Errorf("could not find module %s in graph", moduleName)
	}

	_, distances := graph.ShortestPaths(g, g.moduleIndex[moduleName])

	for i, d := range distances {
		if d >= 0 { // connected node or myself
			module := g.indexIndex[i]
			if module.GetKindStore() == nil {
				continue
			}

			if _, ok := alreadyAdded[module.Name]; ok {
				continue
			}

			res = append(res, module)
			alreadyAdded[module.Name] = true
		}
	}

	sort.Slice(res, func(i, j int) bool {
		return topologicalIndex[res[i].Name] > topologicalIndex[res[j].Name]
	})

	return res, nil
}

func (g *ModuleGraph) GroupedAncestorStores(moduleName string) ([][]*pbsubstreams.Module, error) {
	ancestorStores, err := g.AncestorStoresOf(moduleName)
	if err != nil {
		return nil, fmt.Errorf("getting stores down to %s: %w", moduleName, err)
	}

	distanceMap := map[int64][]*pbsubstreams.Module{}
	distanceIndex := map[*pbsubstreams.Module]int64{}

	_, distances := graph.ShortestPaths(g, g.moduleIndex[moduleName])
	for _, ancestorStore := range ancestorStores {

		for i, d := range distances {
			if g.indexIndex[i].Name == ancestorStore.Name {
				distanceMap[d] = append(distanceMap[d], ancestorStore)
				distanceIndex[ancestorStore] = d
			}
		}
	}

	var result [][]*pbsubstreams.Module
	for _, stores := range distanceMap {
		result = append(result, stores)
	}

	sort.Slice(result, func(i, j int) bool {
		di := distanceIndex[result[i][0]]
		dj := distanceIndex[result[i][0]]
		return di > dj
	})

	return result, nil
}

func (g *ModuleGraph) ParentStoresOf(moduleName string) ([]*pbsubstreams.Modules, error) {
	return nil, nil
}

func (g *ModuleGraph) ModulesDownTo(moduleName string) ([]*pbsubstreams.Module, error) {
	alreadyAdded := map[string]bool{}
	topologicalIndex := map[string]int{}

	sortedModules, ok := g.TopologicalSort()
	if !ok {
		return nil, fmt.Errorf("could not get topological sort of module graph")
	}

	for i, node := range sortedModules {
		topologicalIndex[node.Name] = i
	}

	var res []*pbsubstreams.Module
	if _, found := g.moduleIndex[moduleName]; !found {
		return nil, fmt.Errorf("could not find module %s in graph", moduleName)
	}

	_, distances := graph.ShortestPaths(g, g.moduleIndex[moduleName])

	for i, d := range distances {
		if d >= 0 { // connected node or myself
			module := g.indexIndex[i]
			if _, ok := alreadyAdded[module.Name]; ok {
				continue
			}

			res = append(res, module)
			alreadyAdded[module.Name] = true
		}
	}

	sort.Slice(res, func(i, j int) bool {
		return topologicalIndex[res[i].Name] > topologicalIndex[res[j].Name]
	})

	return res, nil
}

func (g *ModuleGraph) Roots() []*pbsubstreams.Module {
	var roots []*pbsubstreams.Module
	for _, module := range g.modules {
		if g.OutDegree(module.Name) == 0 {
			roots = append(roots, module)
		}
	}
	return roots
}

func (g *ModuleGraph) RootNames() []string {
	var roots []string
	for _, module := range g.Roots() {
		roots = append(roots, module.Name)
	}
	return roots
}

func (g *ModuleGraph) InDegree(moduleName string) int {
	// calculate the in-degree of a module
	moduleIndex, found := g.moduleIndex[moduleName]
	if !found {
		return 0
	}

	var degree int
	for _, v := range g.moduleIndex {
		if v == moduleIndex {
			continue
		}
		if g.Edge(v, moduleIndex) {
			degree++
		}
	}

	return degree
}

func (g *ModuleGraph) Leafs() []*pbsubstreams.Module {
	var leafs []*pbsubstreams.Module
	for _, module := range g.modules {
		if g.InDegree(module.Name) == 0 {
			leafs = append(leafs, module)
		}
	}
	return leafs
}

func (g *ModuleGraph) LeafNames() []string {
	var leafs []string
	for _, module := range g.Leafs() {
		leafs = append(leafs, module.Name)
	}
	return leafs
}

func (g *ModuleGraph) OutDegree(moduleName string) int {
	// calculate the out-degree of a module
	moduleIndex, found := g.moduleIndex[moduleName]
	if !found {
		return 0
	}

	var degree int
	for _, v := range g.moduleIndex {
		if v == moduleIndex {
			continue
		}
		if g.Edge(moduleIndex, v) {
			degree++
		}
	}

	return degree
}

func (g *ModuleGraph) ModuleInitialBlock(moduleName string) (uint64, error) {
	if moduleIndex, found := g.moduleIndex[moduleName]; found {
		return g.modules[moduleIndex].GetInitialBlock(), nil
	}
	return 0, fmt.Errorf("could not find module %s in graph", moduleName)
}

func (g *ModuleGraph) Module(moduleName string) (*pbsubstreams.Module, error) {
	if moduleIndex, found := g.moduleIndex[moduleName]; found {
		return g.modules[moduleIndex], nil
	}
	
	// Module not found, provide helpful suggestions
	return nil, g.createModuleNotFoundError(moduleName)
}

// createModuleNotFoundError creates an enhanced error message with suggestions
func (g *ModuleGraph) createModuleNotFoundError(moduleName string) error {
	// Get all available modules
	allModules := g.modules
	if len(allModules) == 0 {
		return fmt.Errorf("could not find module %q: no modules available in manifest", moduleName)
	}
	
	// Find similar module names using fuzzy matching
	suggestions := g.findSimilarModules(moduleName, allModules)
	
	if len(suggestions) > 0 {
		// Found similar modules, suggest them
		if len(suggestions) == 1 {
			return fmt.Errorf("could not find module %q, did you mean %q?", moduleName, suggestions[0])
		} else {
			return fmt.Errorf("could not find module %q, did you mean one of: %s?", moduleName, strings.Join(suggestions, ", "))
		}
	}
	
	// No similar modules found, list all available output modules
	outputModules := g.getOutputModules(allModules)
	if len(outputModules) == 0 {
		// Fallback to all modules if no output modules found
		moduleNames := make([]string, len(allModules))
		for i, mod := range allModules {
			moduleNames[i] = mod.Name
		}
		return fmt.Errorf("could not find module %q, available modules: %s", moduleName, strings.Join(moduleNames, ", "))
	}
	
	return fmt.Errorf("could not find module %q, available output modules: %s", moduleName, strings.Join(outputModules, ", "))
}

// findSimilarModules finds modules with names similar to the input using fuzzy matching
func (g *ModuleGraph) findSimilarModules(input string, modules []*pbsubstreams.Module) []string {
	const maxSuggestions = 3
	const similarityThreshold = 0.6 // Require at least 60% similarity
	
	type suggestion struct {
		name       string
		similarity float64
	}
	
	var suggestions []suggestion
	
	for _, module := range modules {
		similarity := calculateStringSimilarity(input, module.Name)
		if similarity >= similarityThreshold {
			suggestions = append(suggestions, suggestion{
				name:       module.Name,
				similarity: similarity,
			})
		}
	}
	
	// Sort by similarity (highest first)
	sort.Slice(suggestions, func(i, j int) bool {
		return suggestions[i].similarity > suggestions[j].similarity
	})
	
	// Return top suggestions
	result := make([]string, 0, maxSuggestions)
	for i, s := range suggestions {
		if i >= maxSuggestions {
			break
		}
		result = append(result, s.name)
	}
	
	return result
}

// getOutputModules returns names of modules that can be used as output modules (non-store modules)
func (g *ModuleGraph) getOutputModules(modules []*pbsubstreams.Module) []string {
	var outputModules []string
	
	for _, module := range modules {
		// Skip store modules as they cannot be used as output modules
		if module.GetKindStore() != nil {
			continue
		}
		outputModules = append(outputModules, module.Name)
	}
	
	// Sort alphabetically for consistent output
	sort.Strings(outputModules)
	
	return outputModules
}

// calculateStringSimilarity calculates similarity between two strings using Levenshtein distance
// Returns a value between 0.0 (no similarity) and 1.0 (identical)
func calculateStringSimilarity(s1, s2 string) float64 {
	// Convert to lowercase for case-insensitive comparison
	s1 = strings.ToLower(s1)
	s2 = strings.ToLower(s2)
	
	if s1 == s2 {
		return 1.0
	}
	
	// Calculate Levenshtein distance
	distance := levenshteinDistance(s1, s2)
	maxLen := max(len(s1), len(s2))
	
	if maxLen == 0 {
		return 1.0
	}
	
	// Convert distance to similarity (0.0 to 1.0)
	similarity := 1.0 - float64(distance)/float64(maxLen)
	return similarity
}

// levenshteinDistance calculates the Levenshtein distance between two strings
func levenshteinDistance(s1, s2 string) int {
	if len(s1) == 0 {
		return len(s2)
	}
	if len(s2) == 0 {
		return len(s1)
	}
	
	// Create a matrix to store distances
	matrix := make([][]int, len(s1)+1)
	for i := range matrix {
		matrix[i] = make([]int, len(s2)+1)
	}
	
	// Initialize first row and column
	for i := 0; i <= len(s1); i++ {
		matrix[i][0] = i
	}
	for j := 0; j <= len(s2); j++ {
		matrix[0][j] = j
	}
	
	// Fill the matrix
	for i := 1; i <= len(s1); i++ {
		for j := 1; j <= len(s2); j++ {
			cost := 0
			if s1[i-1] != s2[j-1] {
				cost = 1
			}
			
			matrix[i][j] = min(
				min(matrix[i-1][j]+1, matrix[i][j-1]+1), // deletion, insertion
				matrix[i-1][j-1]+cost,                    // substitution
			)
		}
	}
	
	return matrix[len(s1)][len(s2)]
}

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// max returns the maximum of two integers
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

type ModuleMarshaler []*pbsubstreams.Module

func (m ModuleMarshaler) MarshalJSON() ([]byte, error) {
	l := make([]string, 0, len(m))
	for _, mod := range m {
		l = append(l, mod.Name)
	}

	return json.Marshal(l)
}

func SortModuleNamesByGraphTopology(mods []string, g *ModuleGraph) []string {
	g.TopologicalSort()

	sort.Slice(mods, func(i, j int) bool {
		return g.moduleIndex[mods[i]] < g.moduleIndex[mods[j]]
	})

	return mods
}
