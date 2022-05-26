package manifest

import (
	"bytes"
	"errors"
	"fmt"
	"io/ioutil"
	"math"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"gopkg.in/yaml.v3"
)

const UNSET = math.MaxUint64

var moduleNameRegexp *regexp.Regexp

func init() {
	moduleNameRegexp = regexp.MustCompile(`^([a-zA-Z][a-zA-Z0-9_]{0,63})$`)
}

const (
	ModuleKindStore = "store"
	ModuleKindMap   = "map"
)

// Manifest is a YAML structure used to create a Package and its list
// of Modules. The notion of a manifest does not live in protobuf definitions.
type Manifest struct {
	SpecVersion string      `yaml:"specVersion"` // check that it equals v0.1.0
	Package     PackageMeta `yaml:"package"`
	Protobuf    Protobuf    `yaml:"protobuf"`
	Imports     mapSlice    `yaml:"imports"`
	Modules     []*Module   `yaml:"modules"`

	Graph   *ModuleGraph `yaml:"-"`
	Workdir string       `yaml:"-"`
}

type PackageMeta struct {
	Name    string `yaml:"name"`
	Version string `yaml:"version"` // Semver for package authors
	URL     string `yaml:"url"`
	Doc     string `yaml:"doc"`
}

type Protobuf struct {
	Files       []string `yaml:"files"`
	ImportPaths []string `yaml:"importPaths"`
}

type Module struct {
	Name       string  `yaml:"name"`
	Doc        string  `yaml:"doc"`
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

func loadManifestFile(inputPath string) (*Manifest, error) {
	m, err := decodeYamlManifestFromFile(inputPath)
	if err != nil {
		return nil, fmt.Errorf("decoding yaml: %w", err)
	}

	absoluteManifestPath, err := filepath.Abs(inputPath)
	if err != nil {
		return nil, fmt.Errorf("getting absolute path: %w", err)
	}

	m.Workdir = path.Dir(absoluteManifestPath)

	if m.SpecVersion != "v0.1.0" {
		return nil, fmt.Errorf("invalid 'specVersion', must be v0.1.0")
	}

	// TODO: put some limits on the NUMBER of modules (max 50 ?)
	// TODO: put a limit on the SIZE of the WASM payload (max 10MB per binary?)

	for _, s := range m.Modules {
		// TODO: let's make sure this is also checked when received in Protobuf in a remote request.

		switch s.Kind {
		case ModuleKindMap:
			if s.Output.Type == "" {
				return nil, fmt.Errorf("stream %q: missing 'output.type' for kind 'map'", s.Name)
			}
			// TODO: check protobuf
			if s.Code.Entrypoint == "" {
				s.Code.Entrypoint = "map"
			}
		case ModuleKindStore:
			if err := validateStoreBuilder(s); err != nil {
				return nil, fmt.Errorf("stream %q: %w", s.Name, err)
			}

			if s.Code.Entrypoint == "" {
				// TODO: let's make sure this is validated also when analyzing some incoming protobuf version
				// of this.
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

func decodeYamlManifestFromFile(yamlFilePath string) (out *Manifest, err error) {
	cnt, err := ioutil.ReadFile(yamlFilePath)
	if err != nil {
		return nil, fmt.Errorf("reading substreams manifest %q: %w", yamlFilePath, err)
	}
	if err := yaml.NewDecoder(bytes.NewReader(cnt)).Decode(&out); err != nil {
		return nil, fmt.Errorf("decoding manifest content: %w", err)
	}
	return
}

func (m *Manifest) loadSourceCode() error {
	for _, s := range m.Modules {
		if s.Code.File != "" {
			s.Code.File = path.Join(m.Workdir, s.Code.File)
			cnt, err := ioutil.ReadFile(s.Code.File)
			if err != nil {
				return fmt.Errorf("reading file %q: %w", s.Code.File, err)
			}
			if len(cnt) == 0 {
				return fmt.Errorf("reference wasm file %q empty", s.Code.File)
			}
			s.Code.Content = cnt
		}
	}
	return nil
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
		"max:bigint",               // Exposes SetMaxBigInt
		"max:int64",                // Exposes SetMaxInt64
		"max:bigfloat",             // Exposes SetMaxBigFloat
		"max:float64",              // Exposes SetMaxFloat64
		"min:bigint",               // Exposes SetMinBigInt
		"min:int64",                // Exposes SetMinInt64
		"min:bigfloat",             // Exposes SetMinBigFloat
		"min:float64",              // Exposes SetMinFloat64
		"add:bigint",               // Exposes SumBigInt
		"add:int64",                // Exposes SumInt64
		"add:bigfloat",             // Exposes SumBigFloat
		"add:float64",              // Exposes SubFloat64
		"set:bytes",                // Exposes SetBytes
		"set:string",               // Exposes SetString
		"set:proto",                // Exposes SetBytes
		"set_if_not_exists:bytes",  // Exposes SetBytesIfNotExists
		"set_if_not_exists:string", // Exposes SetStringIfNotExists
		"set_if_not_exists:proto",  // Exposes SetBytesIfNotExists
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

// TODO: there needs to be some safety checks done directly on the _pbsubstreams.Manifest_ object,
// as we'll receive it packaged like that.

func (m *Manifest) loadCode(codePath string, modules *pbsubstreams.Modules) (int, error) {
	byteCode, err := ioutil.ReadFile(codePath)
	if err != nil {
		return 0, fmt.Errorf("reading %q: %w", codePath, err)
	}

	// OPTIM(abourget): also check if it's not already in
	// `ModulesCode`, by comparing its, length + hash or value.

	modules.ModulesCode = append(modules.ModulesCode, byteCode)
	return len(modules.ModulesCode) - 1, nil
}

func (m *Module) String() string {
	return m.Name
}

func (m *Module) ToProtoNative() (*pbsubstreams.Module, error) {
	out := &pbsubstreams.Module{
		Name: m.Name,
		Code: &pbsubstreams.Module_NativeCode_{
			NativeCode: &pbsubstreams.Module_NativeCode{
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

func (m *Module) ToProtoWASM(codeIndex uint32) (*pbsubstreams.Module, error) {
	out := &pbsubstreams.Module{
		Name: m.Name,
		Code: &pbsubstreams.Module_WasmCode_{
			WasmCode: &pbsubstreams.Module_WasmCode{
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

func (m *Module) setInputsToProto(pbModule *pbsubstreams.Module) error {
	for _, input := range m.Inputs {
		if input.Source != "" {
			pbInput := &pbsubstreams.Module_Input{
				Input: &pbsubstreams.Module_Input_Source_{
					Source: &pbsubstreams.Module_Input_Source{
						Type: input.Source,
					},
				},
			}
			pbModule.Inputs = append(pbModule.Inputs, pbInput)
			continue
		}
		if input.Map != "" {
			pbInput := &pbsubstreams.Module_Input{
				Input: &pbsubstreams.Module_Input_Map_{
					Map: &pbsubstreams.Module_Input_Map{
						ModuleName: input.Map,
					},
				},
			}
			pbModule.Inputs = append(pbModule.Inputs, pbInput)
			continue
		}
		if input.Store != "" {

			var mode pbsubstreams.Module_Input_Store_Mode

			switch input.Mode {
			case "":
				mode = pbsubstreams.Module_Input_Store_UNSET
			case "get":
				mode = pbsubstreams.Module_Input_Store_GET
			case "deltas":
				mode = pbsubstreams.Module_Input_Store_DELTAS
			default:
				panic(fmt.Sprintf("invalid input mode %s", input.Mode))
			}

			pbInput := &pbsubstreams.Module_Input{
				Input: &pbsubstreams.Module_Input_Store_{
					Store: &pbsubstreams.Module_Input_Store{
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

const (
	UpdatePolicySet            = "set"
	UpdatePolicySetIfNotExists = "set_if_not_exists"
	UpdatePolicyAdd            = "add"
	UpdatePolicyMax            = "max"
	UpdatePolicyMin            = "min"
)

func (m *Module) setKindToProto(pbModule *pbsubstreams.Module) {
	switch m.Kind {
	case ModuleKindMap:
		pbModule.Kind = &pbsubstreams.Module_KindMap_{
			KindMap: &pbsubstreams.Module_KindMap{
				OutputType: m.Output.Type,
			},
		}
	case ModuleKindStore:
		var updatePolicy pbsubstreams.Module_KindStore_UpdatePolicy
		switch m.UpdatePolicy {
		case UpdatePolicySet:
			updatePolicy = pbsubstreams.Module_KindStore_UPDATE_POLICY_SET
		case UpdatePolicySetIfNotExists:
			updatePolicy = pbsubstreams.Module_KindStore_UPDATE_POLICY_SET_IF_NOT_EXISTS
		case UpdatePolicyAdd:
			updatePolicy = pbsubstreams.Module_KindStore_UPDATE_POLICY_ADD
		case UpdatePolicyMax:
			updatePolicy = pbsubstreams.Module_KindStore_UPDATE_POLICY_MAX
		case UpdatePolicyMin:
			updatePolicy = pbsubstreams.Module_KindStore_UPDATE_POLICY_MIN
		default:
			panic(fmt.Sprintf("invalid update policy %s", m.UpdatePolicy))
		}
		pbModule.Kind = &pbsubstreams.Module_KindStore_{
			KindStore: &pbsubstreams.Module_KindStore{
				UpdatePolicy: updatePolicy,
				ValueType:    m.ValueType,
			},
		}
	}
}

func (m *Module) setOutputToProto(pbModule *pbsubstreams.Module) {
	if m.Output.Type != "" {
		pbModule.Output = &pbsubstreams.Module_Output{
			Type: m.Output.Type,
		}
	}
}
