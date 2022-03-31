package manifest

import (
	"errors"
	"fmt"
	"io/ioutil"
	"math"
	"regexp"
	"strings"

	"github.com/streamingfast/substreams/state"

	"github.com/jhump/protoreflect/desc"
	"github.com/jhump/protoreflect/desc/protoparse"
	pbtransform "github.com/streamingfast/substreams/pb/sf/substreams/transform/v1"
)

const UNSET = math.MaxUint64

var ModuleNameRegexp *regexp.Regexp

func init() {
	ModuleNameRegexp = regexp.MustCompile(`^[a-zA-Z]+[\w]*$`)
}

const (
	ModuleKindStore = "store"
	ModuleKindMap   = "map"
)

type Manifest struct {
	SpecVersion string    `yaml:"specVersion"`
	Description string    `yaml:"description"`
	ProtoFiles  []string  `yaml:"protoFiles"`
	Modules     []*Module `yaml:"modules"`

	Graph      *ModuleGraph           `yaml:"-"`
	ProtoDescs []*desc.FileDescriptor `yaml:"-"`
}

type Module struct {
	Name       string  `yaml:"name"`
	Kind       string  `yaml:"kind"`
	StartBlock *uint64 `yaml:"startBlock"`

	UpdatePolicy string       `yaml:"updatePolicy"`
	ValueType    string       `yaml:"valueType"`
	Code         Code         `yaml:"code"`
	Inputs       []*Input     `yaml:"inputs"`
	Output       StreamOutput `yaml:"output"`
}

type Input struct {
	Source string `yaml:"source"`
	Store  string `yaml:"store"`
	Map    string `yaml:"map"`
	Mode   string `yaml:"mode"`

	Name string `yaml:"-"`
}

type Code struct {
	File       string `yaml:"file"`
	Type       string `yaml:"type"`
	Native     string `yaml:"native"`
	Content    []byte `yaml:"-"`
	Entrypoint string `yaml:"entrypoint"`
}

type StreamOutput struct {
	// For 'map'
	Type string `yaml:"type"`
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

	for _, s := range m.Modules {
		if !ModuleNameRegexp.MatchString(s.Name) {
			return nil, fmt.Errorf("module name %s does not match regex %s", s.Name, ModuleNameRegexp.String())
		}

		switch s.Kind {
		case ModuleKindMap:
			if s.Output.Type == "" {
				return nil, fmt.Errorf("stream %q: missing 'output.type' for kind 'map'", s.Name)
			}
			if s.Code.Entrypoint == "" {
				s.Code.Entrypoint = "map"
			}
		case ModuleKindStore:
			if err := validateStoreBuilder(s); err != nil {
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

func validateStoreBuilder(module *Module) error {
	if module.UpdatePolicy == "" {
		return errors.New("missing 'output.updatePolicy' for kind 'store'")
	}
	if module.ValueType == "" {
		return errors.New("missing 'output.valueType' for kind 'store'")
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
	var lastCombination string
	for _, comb := range combinations {
		valType := module.ValueType
		if strings.HasPrefix(valType, "proto:") {
			valType = "proto"
		}
		lastCombination = fmt.Sprintf("%s:%s", module.UpdatePolicy, valType)
		if lastCombination == comb {
			found = true
		}
	}

	if !found {
		return fmt.Errorf("invalid 'output.updatePolicy' and 'output.valueType' combination, found %q use one of: %s", lastCombination, combinations)
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

func (m *Manifest) ToProto() (*pbtransform.Manifest, error) {
	pbManifest := &pbtransform.Manifest{
		SpecVersion: m.SpecVersion,
		Description: m.Description,
	}

	moduleCodeIndexes := map[string]int{}
	//todo: load wasm code and keep a map of the index
	for _, module := range m.Modules {

		switch module.Code.Type {
		case "native":
			modProto, err := module.ToProtoNative()
			if err != nil {
				return nil, err
			}
			pbManifest.Modules = append(pbManifest.Modules, modProto)

		case "wasm/rust-v1":
			codeIndex, found := moduleCodeIndexes[module.Code.File]
			if !found {
				var err error
				codeIndex, err = m.loadCode(module.Code.File, pbManifest)
				moduleCodeIndexes[module.Code.File] = codeIndex
				if err != nil {
					return nil, fmt.Errorf("loading code: %w", err)
				}
			}

			pbModule, err := module.ToProtoWASM(uint32(codeIndex))
			if err != nil {
				return nil, fmt.Errorf("converting mondule, %s: %w", module.Name, err)
			}
			pbManifest.Modules = append(pbManifest.Modules, pbModule)
		default:
			return nil, fmt.Errorf("invalid code type, %s for module %s", module.Code.Type, module.Name)
		}

	}

	return pbManifest, nil
}

func (m *Manifest) loadCode(codePath string, pbManifest *pbtransform.Manifest) (int, error) {
	byteCode, err := ioutil.ReadFile(codePath)
	if err != nil {
		return 0, fmt.Errorf("reading code from file, %s: %w", codePath, err)
	}

	pbManifest.ModulesCode = append(pbManifest.ModulesCode, byteCode)
	return len(pbManifest.ModulesCode) - 1, nil
}

func (m *Module) String() string {
	return m.Name
}

func (m *Module) ToProtoNative() (*pbtransform.Module, error) {
	out := &pbtransform.Module{
		Name: m.Name,
		Code: &pbtransform.Module_NativeCode{
			NativeCode: &pbtransform.NativeCode{
				Entrypoint: m.Code.Entrypoint,
			},
		},
	}

	out.StartBlock = UNSET
	if m.StartBlock != nil {
		out.StartBlock = *m.StartBlock
	}

	m.setOutputToProto(out)
	m.setKindToProto(out)
	err := m.setInputsToProto(out)
	if err != nil {
		return nil, fmt.Errorf("setting input for module, %s: %w", m.Name, err)
	}
	return out, nil
}

func (m *Module) ToProtoWASM(codeIndex uint32) (*pbtransform.Module, error) {
	out := &pbtransform.Module{
		Name: m.Name,
		Code: &pbtransform.Module_WasmCode{
			WasmCode: &pbtransform.WasmCode{
				Type:       m.Code.Type,
				Index:      codeIndex,
				Entrypoint: m.Code.Entrypoint,
			},
		},
	}

	out.StartBlock = UNSET
	if m.StartBlock != nil {
		out.StartBlock = *m.StartBlock
	}

	m.setOutputToProto(out)
	m.setKindToProto(out)
	err := m.setInputsToProto(out)
	if err != nil {
		return nil, fmt.Errorf("setting input for module, %s: %w", m.Name, err)
	}

	return out, nil
}

func (m *Module) setInputsToProto(pbModule *pbtransform.Module) error {
	for _, input := range m.Inputs {
		if input.Source != "" {
			pbInput := &pbtransform.Input{
				Input: &pbtransform.Input_Source{
					Source: &pbtransform.InputSource{
						Type: input.Source,
					},
				},
			}
			pbModule.Inputs = append(pbModule.Inputs, pbInput)
			continue
		}
		if input.Map != "" {
			pbInput := &pbtransform.Input{
				Input: &pbtransform.Input_Map{
					Map: &pbtransform.InputMap{
						ModuleName: input.Map,
					},
				},
			}
			pbModule.Inputs = append(pbModule.Inputs, pbInput)
			continue
		}
		if input.Store != "" {

			var mode pbtransform.InputStore_Mode

			switch input.Mode {
			case "UNSET":
				mode = pbtransform.InputStore_UNSET
			case "GET":
				mode = pbtransform.InputStore_GET
			case "Delta":
				mode = pbtransform.InputStore_DELTAS
			}

			pbInput := &pbtransform.Input{
				Input: &pbtransform.Input_Store{
					Store: &pbtransform.InputStore{
						ModuleName: input.Store,
						Mode:       mode,
					},
				},
			}
			pbModule.Inputs = append(pbModule.Inputs, pbInput)
			continue
		}

		return fmt.Errorf("invalid input")
	}

	return nil
}

func (m *Module) setKindToProto(pbModule *pbtransform.Module) {
	switch m.Kind {
	case ModuleKindMap:
		pbModule.Kind = &pbtransform.Module_KindMap{
			KindMap: &pbtransform.KindMap{
				OutputType: m.Output.Type,
			},
		}
	case ModuleKindStore:
		var updatePolicy pbtransform.KindStore_UpdatePolicy
		switch m.UpdatePolicy {
		case state.UpdatePolicyReplace:
			updatePolicy = pbtransform.KindStore_UPDATE_POLICY_REPLACE
		case state.UpdatePolicyIgnore:
			updatePolicy = pbtransform.KindStore_UPDATE_POLICY_IGNORE
		case state.UpdatePolicySum:
			updatePolicy = pbtransform.KindStore_UPDATE_POLICY_SUM
		case state.UpdatePolicyMax:
			updatePolicy = pbtransform.KindStore_UPDATE_POLICY_MAX
		case state.UpdatePolicyMin:
			updatePolicy = pbtransform.KindStore_UPDATE_POLICY_MIN
		default:
			panic(fmt.Sprintf("invalid update policy %s", m.UpdatePolicy))
		}
		pbModule.Kind = &pbtransform.Module_KindStore{
			KindStore: &pbtransform.KindStore{
				UpdatePolicy: updatePolicy,
				ValueType:    m.ValueType,
			},
		}
	}
}

func (m *Module) setOutputToProto(pbModule *pbtransform.Module) {
	if m.Output.Type != "" {
		pbModule.Output = &pbtransform.Output{
			Type: m.Output.Type,
		}
	}
}

// TODO FIXME good luck have fun
//type stringer interface {
//	String() string
//}
//
//func MonduleSignature(graph *ModuleGraph, m *pbtransform.Module) []byte {
//	buf := bytes.NewBuffer(nil)
//	buf.WriteString(m.Kind.(stringer).String())
//
//
//	buf.Write(m.Code.Content)
//	buf.Write([]byte(m.Code.Entrypoint))
//
//	sort.Slice(m.Inputs, func(i, j int) bool {
//		return m.Inputs[i].Name < m.Inputs[j].Name
//	})
//	for _, input := range m.Inputs {
//		buf.WriteString(input.Name)
//	}
//
//	ancestors, _ := graph.AncestorsOf(m.Name)
//	for _, ancestor := range ancestors {
//		sig := ancestor.MonduleSignature(graph)
//		buf.Write(sig)
//	}
//
//	h := sha1.New()
//	h.Write(buf.Bytes())
//
//	return h.Sum(nil)
//}
