package manifest

import (
	"bytes"
	"errors"
	"fmt"
	"io/ioutil"
	"math"
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
	SpecVersion string            `yaml:"specVersion"` // check that it equals v0.1.0
	Package     PackageMeta       `yaml:"package"`
	Protobuf    Protobuf          `yaml:"protobuf"`
	Imports     mapSlice          `yaml:"imports"`
	Binaries    map[string]Binary `yaml:"binaries"`
	Modules     []*Module         `yaml:"modules"`

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
	Name         string  `yaml:"name"`
	Doc          string  `yaml:"doc"`
	Kind         string  `yaml:"kind"`
	InitialBlock *uint64 `yaml:"initialBlock"`

	UpdatePolicy string `yaml:"updatePolicy"`
	ValueType    string `yaml:"valueType"`
	Binary       string `yaml:"binary"`
	//Code         Code         `yaml:"code"`
	Inputs []*Input     `yaml:"inputs"`
	Output StreamOutput `yaml:"output"`
}

type Input struct {
	Source string `yaml:"source"`
	Store  string `yaml:"store"`
	Map    string `yaml:"map"`
	Mode   string `yaml:"mode"`

	Name string `yaml:"-"`
}

type Binary struct {
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
func (i *Input) isMap() bool {
	return i.Map != "" && i.Store == "" && i.Source == ""
}
func (i *Input) isStore() bool {
	return i.Store != "" && i.Map == "" && i.Source == ""
}
func (i *Input) isSource() bool {
	return i.Source != "" && i.Map == "" && i.Store == ""
}
func (i *Input) parse() error {
	if i.isMap() {
		i.Name = fmt.Sprintf("map:%s", i.Map)
		return nil
	}
	if i.isStore() {
		i.Name = fmt.Sprintf("store:%s", i.Store)
		if i.Mode == "" {
			i.Mode = "get"
		}
		if i.Mode != "get" && i.Mode != "deltas" {
			return fmt.Errorf("input %q: 'mode' parameter must be one of: 'get', 'deltas'", i.Name)
		}
		return nil
	}
	if i.isSource() {
		i.Name = fmt.Sprintf("source:%s", i.Source)
		return nil
	}
	return fmt.Errorf("input has an unknown type. Expect one, and only one of 'map', 'store' or 'source'")
}

func validateStoreBuilder(module *Module) error {
	if module.UpdatePolicy == "" {
		return errors.New("missing 'output.updatePolicy' for kind 'store'")
	}
	if module.ValueType == "" {
		return errors.New("missing 'output.valueType' for kind 'store'")
	}

	combinations := []string{
		"max:bigint",
		"max:int64",
		"max:bigfloat",
		"max:float64",
		"min:bigint",
		"min:int64",
		"min:bigfloat",
		"min:float64",
		"add:bigint",
		"add:int64",
		"add:bigfloat",
		"add:float64",
		"set:bytes",
		"set:string",
		"set:proto",
		"set_if_not_exists:bytes",
		"set_if_not_exists:string",
		"set_if_not_exists:proto",
		"append:bytes",
		"append:string",
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

func (m *Module) String() string {
	return m.Name
}

func (m *Module) ToProtoWASM(codeIndex uint32) (*pbsubstreams.Module, error) {
	out := &pbsubstreams.Module{
		Name:             m.Name,
		BinaryIndex:      codeIndex,
		BinaryEntrypoint: m.Name,
	}

	out.InitialBlock = UNSET
	if m.InitialBlock != nil {
		out.InitialBlock = *m.InitialBlock
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
	UpdatePolicyAppend         = "append"
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
		case UpdatePolicyAppend:
			updatePolicy = pbsubstreams.Module_KindStore_UPDATE_POLICY_APPEND
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
