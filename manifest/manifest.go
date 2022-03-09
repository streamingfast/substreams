package manifest

import (
	"bytes"
	"crypto/sha1"
	"errors"
	"fmt"
	"io/ioutil"
	"sort"
	"strings"
)

type Manifest struct {
	SpecVersion  string    `yaml:"specVersion"`
	Description  string    `yaml:"description"`
	CodeType     string    `yaml:"codeType"`
	GenesisBlock int       `yaml:"genesisBlock"`
	Modules      []*Module `yaml:"modules"`

	Graph *StreamsGraph `yaml:"-"`
}

type Module struct {
	Name   string       `yaml:"name"`
	Kind   string       `yaml:"kind"`
	Code   Code         `yaml:"code"`
	Inputs []*Input     `yaml:"inputs"`
	Output StreamOutput `yaml:"output"`
}

type Input struct {
	// source, store, and map are mutually exclusive
	// mode must be set only if "store" is set
	// mode must be one of "get", "deltas
	Source string `yaml:"source"`
	Store  string `yaml:"store"`
	Map    string `yaml:"map"`
	Mode   string `yaml:"mode"`

	Name string `yaml:"-"`
}

func (i *Input) parse() error {
	if i.Map != "" && i.Store == "" && i.Source == "" {
		i.Name = fmt.Sprintf("map:%s", i.Map)
		return nil
	}
	if i.Store != "" && i.Map == "" && i.Source == "" {
		i.Name = fmt.Sprintf("store:%s", i.Store)
		if i.Mode == "" {
			i.Mode = "get"
		}
		if i.Mode != "get" && i.Mode != "deltas" {
			return fmt.Errorf("input %q: 'mode' parameter must be one of: 'get', 'deltas'", i.Name)
		}
		return nil
	}
	if i.Source != "" && i.Map == "" && i.Store == "" {
		i.Name = fmt.Sprintf("source:%s", i.Source)
		return nil
	}
	return fmt.Errorf("one, and only one of 'map', 'store' or 'source' must be specified")
}

type Code struct {
	File       string `yaml:"file"`
	Native     string `yaml:"native"`
	Content    []byte `yaml:"-"`
	Entrypoint string `yaml:"entrypoint"`
}

type StreamOutput struct {
	// For mappers
	Type string `yaml:"type"`

	// For state builders
	ValueType    string `yaml:"valueType"`
	ProtoType    string `yaml:"protoType"` // when `ValueType` == "proto"
	UpdatePolicy string `yaml:"updatePolicy"`
}

func New(path string) (m *Manifest, err error) {
	m, err = newWithoutLoad(path)
	if err != nil {
		return nil, err
	}

	for _, s := range m.Modules {
		for _, input := range s.Inputs {
			if err := input.parse(); err != nil {
				return nil, fmt.Errorf("module %q: %w", s.Name, err)
			}
		}
		if s.Code.File != "" {
			cnt, err := ioutil.ReadFile(s.Code.File)
			if err != nil {
				return nil, fmt.Errorf("reading file %q: %w", s.Code.File, err)
			}
			if len(cnt) == 0 {
				return nil, fmt.Errorf("reference wasm file empty: %s", s.Code.File)
			}
			s.Code.Content = cnt
		}
	}
	return
}

func newWithoutLoad(path string) (*Manifest, error) {
	_, m, err := DecodeYamlManifestFromFile(path)
	if err != nil {
		return nil, fmt.Errorf("decoding yaml: %w", err)
	}

	switch m.CodeType {
	case "wasm/rust-v1", "native":
	default:
		return nil, fmt.Errorf("invalid value %q for 'codeType'", m.CodeType)
	}

	for _, s := range m.Modules {
		switch s.Kind {
		case "map":
			if s.Output.Type == "" {
				return nil, fmt.Errorf("stream %q: missing 'output.type' for kind 'map'", s.Name)
			}
			if s.Code.Entrypoint == "" {
				s.Code.Entrypoint = "map"
			}
		case "store":
			if err := validateStoreBuilderOutput(s.Output); err != nil {
				return nil, fmt.Errorf("stream %q: %w", s.Name, err)
			}

			if s.Code.Entrypoint == "" {
				s.Code.Entrypoint = "build_state"
			}

		default:
			return nil, fmt.Errorf("stream %q: invalid kind %q", s.Name, s.Kind)
		}
	}

	graph, err := NewStreamsGraph(m.Modules)
	if err != nil {
		return nil, fmt.Errorf("computing streams graph: %w", err)
	}

	m.Graph = graph

	return m, nil
}

func validateStoreBuilderOutput(output StreamOutput) error {
	if output.UpdatePolicy == "" {
		return errors.New("missing 'output.updatePolicy' for kind 'store'")
	}
	if output.ValueType == "" {
		return errors.New("missing 'output.valueType' for kind 'store'")
	}
	if output.ValueType == "proto" && output.ProtoType == "" {
		return errors.New("missing 'output.protoType' for kind StateBuidler, required when 'output.valueType' set to 'proto'")
	}

	combinations := []string{
		"max:bigint",     // Exposes SetMaxBigInt
		"max:int64",      // Exposes SetMaxInt64
		"max:bigfloat",   // Exposes SetMaxBigFloat
		"max:float64",    // Exposes SetMaxFloat64
		"min:bigint",     // Exposes SetMinBigInt
		"min:int64",      // Exposes SetMinInt64
		"min:bigfloat",   // Exposes SetMinBigFloat
		"min:float64",    // Exposes SetMinFloat64
		"sum:bigint",     // Exposes SumBigInt
		"sum:int64",      // Exposes SumInt64
		"sum:bigfloat",   // Exposes SumBigFloat
		"sum:float64",    // Exposes SubFloat64
		"replace:bytes",  // Exposes SetBytes
		"replace:string", // Exposes SetString
		"replace:proto",  // Exposes SetBytes
		"ignore:bytes",   // Exposes SetBytesIfNotExists
		"ignore:string",  // Exposes SetStringIfNotExists
		"ignore:proto",   // Exposes SetBytesIfNotExists
	}
	found := false
	for _, comb := range combinations {
		if fmt.Sprintf("%s:%s", output.UpdatePolicy, output.ValueType) == comb {
			found = true
		}
	}

	if !found {
		return fmt.Errorf("invalid 'output.updatePolicy' and 'output.valueType' combination, use one of: %s", combinations)
	}

	return nil
}

func (m *Manifest) PrintMermaid() {
	fmt.Println("Mermaid graph:\n\n```mermaid\ngraph TD;")

	for _, s := range m.Modules {
		for _, in := range s.Inputs {
			fmt.Printf("  %s -- %q --> %s\n", strings.Split(in.Name, ":")[1], in.Name, s.Name)
		}
	}

	fmt.Println("```")
	fmt.Println("")
}

func (s *Module) Signature(graph *StreamsGraph) []byte {
	buf := bytes.NewBuffer(nil)
	buf.WriteString(s.Kind)
	buf.Write(s.Code.Content)
	buf.Write([]byte(s.Code.Entrypoint))

	sort.Slice(s.Inputs, func(i, j int) bool {
		return s.Inputs[i].Name < s.Inputs[j].Name
	})
	for _, input := range s.Inputs {
		buf.WriteString(input.Name)
	}

	ancestors := graph.AncestorsOf(s.Name)
	for _, ancestor := range ancestors {
		sig := ancestor.Signature(graph)
		buf.Write(sig)
	}

	h := sha1.New()
	h.Write(buf.Bytes())

	return h.Sum(nil)
}

func (s *Module) String() string {
	return s.Name
}

type StreamsGraph struct {
	streams map[string]*Module
	links   map[string][]*Module
}

func NewStreamsGraph(streams []*Module) (*StreamsGraph, error) {
	sg := &StreamsGraph{
		streams: map[string]*Module{},
		links:   map[string][]*Module{},
	}

	for _, stream := range streams {
		sg.streams[stream.Name] = stream
	}

	for _, stream := range streams {
		var links []*Module
		for _, input := range stream.Inputs {
			for _, streamPrefix := range []string{"map:", "store:"} {
				if strings.HasPrefix(input.Name, streamPrefix) {
					linkName := strings.TrimPrefix(input.Name, streamPrefix)
					linkedStream, ok := sg.streams[linkName]
					if !ok {
						return nil, fmt.Errorf("%s does not exist", linkName)
					}
					links = append(links, linkedStream)
				}
			}

		}
		sg.links[stream.Name] = links
	}

	return sg, nil
}

func (g *StreamsGraph) StreamsFor(streamName string) ([]*Module, error) {
	thisStream, found := g.streams[streamName]
	if !found {
		return nil, fmt.Errorf("stream %q not found", streamName)
	}

	parents := g.ancestorsOf(streamName)
	return append(parents, thisStream), nil
}

//TODO: use this in pipeline and deduplicate everything
func (g *StreamsGraph) GroupedStreamsFor(streamName string) ([][]*Module, error) {
	thisStream, found := g.streams[streamName]
	if !found {
		return nil, fmt.Errorf("stream %q not found", streamName)
	}

	parents := g.groupedAncestorsOf(streamName)
	return append(parents, []*Module{thisStream}), nil
}

func (g *StreamsGraph) AncestorsOf(streamName string) []*Module {
	parents := g.ancestorsOf(streamName)
	return parents
}

func (g *StreamsGraph) GroupedAncestorsOf(streamName string) []*Module {
	parents := g.ancestorsOf(streamName)
	return parents
}

func (g *StreamsGraph) ancestorsOf(streamName string) []*Module {
	type streamWithTreeDepth struct {
		stream *Module
		depth  int
	}

	var dfs func(rootName string, depth int) []streamWithTreeDepth
	dfs = func(rootName string, depth int) []streamWithTreeDepth {
		var result []streamWithTreeDepth
		for _, link := range g.links[rootName] {
			result = append(result, streamWithTreeDepth{
				stream: link,
				depth:  depth,
			})

			result = append(result, dfs(link.Name, depth+1)...)
		}

		return result
	}

	parentsWithDepth := dfs(streamName, 0)

	//sort by depth in descending order
	sort.Slice(parentsWithDepth, func(i, j int) bool {
		//tie break alphabetically by name
		if parentsWithDepth[i].depth == parentsWithDepth[j].depth {
			return parentsWithDepth[i].stream.Name < parentsWithDepth[j].stream.Name
		}
		return parentsWithDepth[i].depth > parentsWithDepth[j].depth
	})

	seen := map[string]struct{}{}
	var result []*Module
	for _, parent := range parentsWithDepth {
		if _, ok := seen[parent.stream.Name]; ok {
			continue
		}
		result = append(result, parent.stream)
		seen[parent.stream.Name] = struct{}{}
	}

	return result
}

func (g *StreamsGraph) groupedAncestorsOf(streamName string) [][]*Module {
	type streamWithTreeDepth struct {
		stream *Module
		depth  int
	}

	var dfs func(rootName string, depth int) []streamWithTreeDepth
	dfs = func(rootName string, depth int) []streamWithTreeDepth {
		var result []streamWithTreeDepth
		for _, link := range g.links[rootName] {
			result = append(result, streamWithTreeDepth{
				stream: link,
				depth:  depth,
			})

			result = append(result, dfs(link.Name, depth+1)...)
		}

		return result
	}

	parentsWithDepth := dfs(streamName, 0)

	//sort by depth in descending order
	sort.Slice(parentsWithDepth, func(i, j int) bool {
		//tie break alphabetically by name
		if parentsWithDepth[i].depth == parentsWithDepth[j].depth {
			return parentsWithDepth[i].stream.Name < parentsWithDepth[j].stream.Name
		}
		return parentsWithDepth[i].depth > parentsWithDepth[j].depth
	})

	grouped := map[int][]*Module{}
	seen := map[string]struct{}{}
	for _, parent := range parentsWithDepth {
		if _, ok := seen[parent.stream.Name]; ok {
			continue
		}
		grouped[parent.depth] = append(grouped[parent.depth], parent.stream)
		seen[parent.stream.Name] = struct{}{}
	}

	result := make([][]*Module, len(grouped), len(grouped))
	for i, streams := range grouped {
		result[len(grouped)-1-i] = streams
	}

	return result
}
