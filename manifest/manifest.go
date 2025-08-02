package manifest

import (
	"bytes"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

const UNSET = math.MaxUint64

var moduleNameRegexp *regexp.Regexp

func init() {
	moduleNameRegexp = regexp.MustCompile(`^([a-zA-Z][a-zA-Z0-9_-]{0,63})$`)
}

const (
	ModuleKindStore      = "store"
	ModuleKindMap        = "map"
	ModuleKindBlockIndex = "blockIndex"
)

// Manifest is a YAML structure used to create a Package and its list
// of Modules. The notion of a manifest does not live in protobuf definitions.
type Manifest struct {
	SpecVersion  string                    `yaml:"specVersion,omitempty"` // check that it equals v0.1.0
	Package      PackageMeta               `yaml:"package,omitempty"`
	Protobuf     Protobuf                  `yaml:"protobuf,omitempty"`
	Imports      mapSlice                  `yaml:"imports,omitempty"`
	Binaries     map[string]Binary         `yaml:"binaries,omitempty"`
	Modules      []*Module                 `yaml:"modules,omitempty"`
	Params       map[string]string         `yaml:"params,omitempty"`
	BlockFilters map[string]string         `yaml:"blockFilters,omitempty"`
	Network      string                    `yaml:"network,omitempty"`
	Networks     map[string]*NetworkParams `yaml:"networks,omitempty"`
	Sink         *Sink                     `yaml:"sink,omitempty"`

	Graph   *ModuleGraph `yaml:"-"`
	Workdir string       `yaml:"-"`
}

type NetworkParams struct {
	InitialBlocks map[string]uint64 `yaml:"initialBlock,omitempty" json:"initialBlock,omitempty"`
	Params        map[string]string `yaml:"params,omitempty" json:"params,omitempty"`
}

type Sink struct {
	Type   string      `yaml:"type,omitempty"`
	Module string      `yaml:"module,omitempty"`
	Config interface{} `yaml:"config,omitempty"`
}

var httpSchemePrefixRegex = regexp.MustCompile("^https?://")

func (m *Manifest) resolvePath(path string) string {
	if m.Workdir == "" || filepath.IsAbs(path) || httpSchemePrefixRegex.MatchString(path) {
		return path
	}

	return filepath.Join(m.Workdir, path)
}

type PackageMeta struct {
	Name        string `yaml:"name,omitempty"`
	Version     string `yaml:"version,omitempty"` // Semver for package authors
	URL         string `yaml:"url,omitempty"`
	Doc         string `yaml:"doc,omitempty"`
	Description string `yaml:"description,omitempty"`
	Image       string `yaml:"image,omitempty"`
}

type Protobuf struct {
	DescriptorSets []*BufImport `yaml:"descriptorSets,omitempty"`
	Files          []string     `yaml:"files,omitempty"`
	ImportPaths    []string     `yaml:"importPaths,omitempty"`
	ExcludePaths   []string     `yaml:"excludePaths,omitempty"`
}

type BufImport struct {
	LocalPath string   `yaml:"localPath,omitempty"`
	Module    string   `yaml:"module"`
	Version   string   `yaml:"version"`
	Symbols   []string `yaml:"symbols"`
}

type Module struct {
	Name         string       `yaml:"name,omitempty"`
	Doc          string       `yaml:"doc,omitempty"`
	Kind         string       `yaml:"kind,omitempty"`
	InitialBlock *uint64      `yaml:"initialBlock,omitempty"`
	BlockFilter  *BlockFilter `yaml:"blockFilter,omitempty"`

	UpdatePolicy string `yaml:"updatePolicy,omitempty"`
	ValueType    string `yaml:"valueType,omitempty"`
	Binary       string `yaml:"binary,omitempty"`

	Inputs []*Input     `yaml:"inputs,omitempty"`
	Output StreamOutput `yaml:"output,omitempty"`
	Use    string       `yaml:"use,omitempty"`
}

type BlockFilter struct {
	Module string           `yaml:"module,omitempty"`
	Query  BlockFilterQuery `yaml:"query,omitempty"`
}

func (bf *BlockFilter) IsEmpty() bool {
	return bf.Module == "" && bf.Query.String == "" && !bf.Query.Params
}

type BlockFilterQuery struct {
	String string `yaml:"string,omitempty"`
	Params bool   `yaml:"params,omitempty"`
	// Store string `yaml:"store,omitempty"`
}

type Input struct {
	Source string `yaml:"source,omitempty"`
	Store  string `yaml:"store,omitempty"`
	Map    string `yaml:"map,omitempty"`
	Params string `yaml:"params,omitempty"`

	Mode string `yaml:"mode,omitempty"`
}

type Binary struct {
	File                string            `yaml:"file,omitempty"`
	Type                string            `yaml:"type,omitempty"`
	Native              string            `yaml:"native,omitempty"`
	Content             []byte            `yaml:"-"`
	Entrypoint          string            `yaml:"entrypoint,omitempty"`
	ProtoPackageMapping map[string]string `yaml:"protoPackageMapping,omitempty"`
	Build               string            `yaml:"build,omitempty"`
}

type StreamOutput struct {
	// For 'map'
	Type string `yaml:"type,omitempty"`
}

func decodeYamlManifestFromFile(yamlFilePath, workingDir string) (out *Manifest, err error) {
	//if yamlFilePath is a relative path, make it absolute
	if !filepath.IsAbs(yamlFilePath) {
		yamlFilePath = filepath.Join(workingDir, yamlFilePath)
	}

	cnt, err := os.ReadFile(yamlFilePath)
	if err != nil {
		return nil, fmt.Errorf("reading substreams manifest %q: %w", yamlFilePath, err)
	}

	decoder := yaml.NewDecoder(bytes.NewReader(cnt))
	decoder.KnownFields(true)
	if err := decoder.Decode(&out); err != nil {
		return nil, fmt.Errorf("decoding manifest content: %w", err)
	}

	return
}

func (i *Input) IsMap() bool {
	return i.Map != "" && i.Store == "" && i.Source == "" && i.Params == ""
}

func (i *Input) IsStore() bool {
	return i.Store != "" && i.Map == "" && i.Source == "" && i.Params == ""
}

func (i *Input) IsSource() bool {
	return i.Source != "" && i.Map == "" && i.Store == "" && i.Params == ""
}

func (i *Input) IsParams() bool {
	return i.Params != "" && i.Source == "" && i.Map == "" && i.Store == ""
}

func (i *Input) parse() error {
	if i.IsMap() {
		//i.Name = fmt.Sprintf("map:%s", i.Map)
		return nil
	}
	if i.IsStore() {
		if i.Mode == "" {
			i.Mode = "get"
		}
		if i.Mode != "get" && i.Mode != "deltas" {
			return fmt.Errorf("input store %q: 'mode' parameter must be one of: 'get', 'deltas'", i.Store)
		}
		return nil
	}
	if i.IsSource() {
		return nil
	}
	if i.IsParams() {
		if i.Params != "string" {
			return fmt.Errorf("input 'params': 'string' is the only acceptable value here; specify the parameter's value under the top-level 'params' mapping")
		}
		return nil
	}
	return fmt.Errorf("input has an unknown or mixed types; expect one, and only one of: 'params', 'map', 'store' or 'source'")
}

func validateModuleWithUse(module *Module) error {
	if module.Output.Type != "" {
		return fmt.Errorf("module %q: 'output.type' cannot be set when 'use' is set", module.Name)
	}

	if module.UpdatePolicy != "" {
		return fmt.Errorf("module %q: 'output.updatePolicy' cannot be set when 'use' is set", module.Name)
	}

	if module.Binary != "" {
		return fmt.Errorf("module %q: 'binary' cannot be set when 'use' is set", module.Name)
	}

	if module.ValueType != "" {
		return fmt.Errorf("module %q: 'valueType' cannot be set when 'use' is set", module.Name)
	}

	return nil
}

func validateStoreBuilder(module *Module) error {
	if module.UpdatePolicy == "" {
		return errors.New("missing 'output.updatePolicy' for kind 'store'")
	}
	if module.ValueType == "" {
		return errors.New("missing 'output.valueType' for kind 'store'")
	}

	// keep big float to be backward-compatible
	combinations := []string{
		"max:bigint",
		"max:int64",
		"max:bigdecimal",
		"max:bigfloat",
		"max:float64",
		"min:bigint",
		"min:int64",
		"min:bigdecimal",
		"min:bigfloat",
		"min:float64",
		"add:bigint",
		"add:int64",
		"add:bigdecimal",
		"add:bigfloat",
		"add:float64",
		"set:bytes",
		"set:string",
		"set:proto",
		"set:bigdecimal",
		"set:bigfloat",
		"set:bigint",
		"set:int64",
		"set:float64",
		"set_if_not_exists:bytes",
		"set_if_not_exists:string",
		"set_if_not_exists:proto",
		"set_if_not_exists:bigdecimal",
		"set_if_not_exists:bigfloat",
		"set_if_not_exists:bigint",
		"set_if_not_exists:int64",
		"set_if_not_exists:float64",
		"set_sum:bigint",
		"set_sum:int64",
		"set_sum:bigdecimal",
		"set_sum:float64",
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

	m.setBlockFilterToProto(out)

	err := m.setInputsToProto(out)
	if err != nil {
		return nil, fmt.Errorf("setting input for module, %s: %w", m.Name, err)
	}

	return out, nil
}

func (m *Module) setBlockFilterToProto(pbModule *pbsubstreams.Module) {
	if m.BlockFilter != nil {
		bf := &pbsubstreams.Module_BlockFilter{
			Module: m.BlockFilter.Module,
		}
		switch {
		case m.BlockFilter.Query.String != "":
			bf.Query = &pbsubstreams.Module_BlockFilter_QueryString{
				QueryString: m.BlockFilter.Query.String,
			}
		case m.BlockFilter.Query.Params:
			bf.Query = &pbsubstreams.Module_BlockFilter_QueryFromParams{}
		}

		pbModule.BlockFilter = bf
	}
}

func (m *Module) setInputsToProto(pbModule *pbsubstreams.Module) error {
	for i, input := range m.Inputs {
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
		if input.Params != "" {
			if i != 0 {
				return fmt.Errorf("input.params must be the first input")
			}

			pbInput := &pbsubstreams.Module_Input{
				Input: &pbsubstreams.Module_Input_Params_{
					Params: &pbsubstreams.Module_Input_Params{
						Value: "",
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
	OutputValueTypeInt64      = "int64"
	OutputValueTypeFloat64    = "float64"
	OutputValueTypeBigInt     = "bigint"
	OutputValueTypeBigDecimal = "bigdecimal"

	// Deprecated: bigfloat value type replaced with bigdecimal
	OutputValueTypeBigFloat = "bigfloat"
	OutputValueTypeString   = "string"
)

const (
	UpdatePolicySet            = "set"
	UpdatePolicySetIfNotExists = "set_if_not_exists"
	UpdatePolicyAdd            = "add"
	UpdatePolicyMax            = "max"
	UpdatePolicyMin            = "min"
	UpdatePolicyAppend         = "append"
	UpdatePolicySetSum         = "set_sum"
)

func (m *Module) setKindToProto(pbModule *pbsubstreams.Module) {
	switch m.Kind {
	case ModuleKindBlockIndex:
		pbModule.Kind = &pbsubstreams.Module_KindBlockIndex_{
			KindBlockIndex: &pbsubstreams.Module_KindBlockIndex{
				OutputType: m.Output.Type,
			},
		}
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
		case UpdatePolicySetSum:
			updatePolicy = pbsubstreams.Module_KindStore_UPDATE_POLICY_SET_SUM
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
