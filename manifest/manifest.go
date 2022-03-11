package manifest

import (
	"bytes"
	"crypto/sha1"
	"errors"
	"fmt"
	"io/ioutil"
	"regexp"
	"sort"
	"strings"

	"github.com/jhump/protoreflect/desc"
	"github.com/jhump/protoreflect/desc/protoparse"
)

var ModuleNameRegexp *regexp.Regexp

func init() {
	ModuleNameRegexp = regexp.MustCompile(`^[a-zA-Z]+[\w\d]*$`)
}

type Manifest struct {
	SpecVersion string    `yaml:"specVersion"`
	Description string    `yaml:"description"`
	CodeType    string    `yaml:"codeType"`
	StartBlock  uint64    `yaml:"startBlock"` // TODO: This needs to go on the actual module, perhaps can be inferred from its dependencies
	ProtoFiles  []string  `yaml:"protoFiles"`
	Modules     []*Module `yaml:"modules"`

	Graph      *ModuleGraph           `yaml:"-"`
	ProtoDescs []*desc.FileDescriptor `yaml:"-"`
}

type Module struct {
	Name   string       `yaml:"name"`
	Kind   string       `yaml:"kind"`
	Code   Code         `yaml:"code"`
	Inputs []*Input     `yaml:"inputs"`
	Output StreamOutput `yaml:"output"`
}

type Input struct {
	// TODO: implement the checks to enforce these clauses:
	// * source, store, and map are mutually exclusive
	// * mode must be set only if "store" is set
	// * mode must be one of "get", "deltas
	Source string `yaml:"source"`
	Store  string `yaml:"store"`
	Map    string `yaml:"map"`
	Mode   string `yaml:"mode"`

	Name string `yaml:"-"`
}

type Code struct {
	File       string `yaml:"file"`
	Native     string `yaml:"native"`
	Content    []byte `yaml:"-"`
	Entrypoint string `yaml:"entrypoint"`
}

type StreamOutput struct {
	// For 'map'
	Type string `yaml:"type"`

	// For 'store'
	ValueType    string `yaml:"valueType"`
	ProtoType    string `yaml:"protoType"` // when `ValueType` == "proto"
	UpdatePolicy string `yaml:"updatePolicy"`
}

func New(path string) (m *Manifest, err error) {
	m, err = newWithoutLoad(path)
	if err != nil {
		return nil, err
	}

	parser := protoparse.Parser{}
	fileDescs, err := parser.ParseFiles(m.ProtoFiles...)
	if err != nil {
		return nil, fmt.Errorf("error parsing proto files %q: %w", m.ProtoFiles, err)
	}
	m.ProtoDescs = fileDescs

	for _, s := range m.Modules {
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
		if !ModuleNameRegexp.MatchString(s.Name) {
			return nil, fmt.Errorf("module name %s does not match regex %s", s.Name, ModuleNameRegexp.String())
		}

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

		for _, input := range s.Inputs {
			if err := input.parse(); err != nil {
				return nil, fmt.Errorf("module %q: %w", s.Name, err)
			}
		}
	}

	graph, err := NewModuleGraph(m.Modules)
	if err != nil {
		return nil, fmt.Errorf("computing modules graph: %w", err)
	}

	m.Graph = graph

	return m, nil
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
			dataPassed := in.Name
			if in.Mode != "" {
				dataPassed = dataPassed + ":" + in.Mode
			}
			fmt.Printf("  %s -- %q --> %s\n", strings.Split(in.Name, ":")[1], dataPassed, s.Name)
		}
	}

	fmt.Println("```")
	fmt.Println("")
}

func (s *Module) Signature(graph *ModuleGraph) []byte {
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

	ancestors, _ := graph.AncestorsOf(s.Name)
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
