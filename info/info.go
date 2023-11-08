package info

import (
	"fmt"
	"strings"

	"github.com/streamingfast/substreams/manifest"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/pipeline/outputmodules"
	"google.golang.org/protobuf/types/descriptorpb"
)

type BasicInfo struct {
	Name                   string                         `json:"name"`
	Version                string                         `json:"version"`
	Documentation          *string                        `json:"documentation,omitempty"`
	Network                string                         `json:"network,omitempty"`
	Image                  []byte                         `json:"-"`
	Modules                []ModulesInfo                  `json:"modules"`
	SinkInfo               *SinkInfo                      `json:"sink_info,omitempty"`
	ProtoPackages          []string                       `json:"proto_packages"`            // list of proto packages
	ProtoSourceCode        map[string][]*SourceCodeInfo   `json:"proto_source_code"`         // map of proto file name to .proto file contents
	ProtoMessagesByPackage map[string][]*ProtoMessageInfo `json:"proto_messages_by_package"` // map of package name to a list of messages info in that package
}

type SourceCodeInfo struct {
	Filename string `json:"filename"`
	Source   string `json:"source"`
}

type ProtoMessageInfo struct {
	Name           string              `json:"name"`
	Package        string              `json:"package"`
	Type           string              `json:"type"`
	File           string              `json:"file"`
	Proto          string              `json:"proto"`
	Documentation  string              `json:"documentation"`
	NestedMessages []*ProtoMessageInfo `json:"nested_messages"`
}

type SinkInfo struct {
	Configs string            `json:"desc"`
	TypeUrl string            `json:"type_url"`
	Files   map[string][]byte `json:"files"`
}

type ExtendedInfo struct {
	*BasicInfo

	ExecutionStages [][][]string `json:"execution_stages,omitempty"`
}

type ProtoFileInfo struct {
	Name               *string                                `json:"name,omitempty"`
	Package            *string                                `json:"package,omitempty"`
	Dependencies       []string                               `json:"dependencies,omitempty"`
	PublicDependencies []int32                                `json:"public_dependencies,omitempty"`
	MessageType        []*descriptorpb.DescriptorProto        `json:"message_type,omitempty"`
	Services           []*descriptorpb.ServiceDescriptorProto `json:"services,omitempty"`
}

type ModulesInfo struct {
	Name          string        `json:"name"`
	Kind          string        `json:"kind"`
	Inputs        []ModuleInput `json:"inputs"`
	OutputType    *string       `json:"output_type,omitempty"`   //for map inputs
	ValueType     *string       `json:"value_type,omitempty"`    //for store inputs
	UpdatePolicy  *string       `json:"update_policy,omitempty"` //for store inputs
	InitialBlock  uint64        `json:"initial_block"`
	Documentation *string       `json:"documentation,omitempty"`
	Hash          string        `json:"hash"`
}

type ModuleInput struct {
	Type string  `json:"type"`
	Name string  `json:"name"`
	Mode *string `json:"mode,omitempty"` //for store inputs
}

func Basic(pkg *pbsubstreams.Package) (*BasicInfo, error) {
	name := "Unnamed"
	var doc, version string
	if len(pkg.PackageMeta) != 0 {
		name = pkg.PackageMeta[0].Name
		version = pkg.PackageMeta[0].Version
		doc = pkg.PackageMeta[0].Doc
	}

	manifestInfo := &BasicInfo{
		Name:    name,
		Network: pkg.Network,
		Version: version,
		Image:   pkg.Image,
	}
	if doc != "" {
		manifestInfo.Documentation = strPtr(strings.Replace(doc, "\n", "\n  ", -1))
	}

	graph, err := manifest.NewModuleGraph(pkg.Modules.Modules)
	if err != nil {
		return nil, fmt.Errorf("creating module graph: %w", err)
	}

	modules := make([]ModulesInfo, 0, len(pkg.Modules.Modules))

	hashes := manifest.NewModuleHashes()
	for ix, mod := range pkg.Modules.Modules {
		modInfo := ModulesInfo{}

		_, _ = hashes.HashModule(pkg.Modules, mod, graph)
		modInfo.Hash = hashes.Get(mod.Name)

		modInfo.Name = mod.Name
		modInfo.InitialBlock = mod.InitialBlock

		kind := mod.GetKind()
		switch v := kind.(type) {
		case *pbsubstreams.Module_KindMap_:
			modInfo.Kind = "map"
			modInfo.OutputType = strPtr(v.KindMap.OutputType)
		case *pbsubstreams.Module_KindStore_:
			modInfo.Kind = "store"
			modInfo.ValueType = strPtr(v.KindStore.ValueType)
			modInfo.UpdatePolicy = strPtr(v.KindStore.UpdatePolicy.Pretty())
		default:
			modInfo.Kind = "unknown"
		}

		if pkg.ModuleMeta != nil {
			modMeta := pkg.ModuleMeta[ix]
			if modMeta != nil && modMeta.Doc != "" {
				modInfo.Documentation = strPtr(strings.Replace(modMeta.Doc, "\n", "\n  ", -1))
			}
		}

		inputs := make([]ModuleInput, 0, len(mod.Inputs))
		for _, input := range mod.Inputs {
			inputInfo := ModuleInput{}

			switch v := input.Input.(type) {
			case *pbsubstreams.Module_Input_Source_:
				inputInfo.Type = "source"
				inputInfo.Name = v.Source.Type
			case *pbsubstreams.Module_Input_Map_:
				inputInfo.Type = "map"
				inputInfo.Name = v.Map.ModuleName
			case *pbsubstreams.Module_Input_Store_:
				inputInfo.Type = "store"
				inputInfo.Name = v.Store.ModuleName
				if v.Store.Mode > 0 {
					inputInfo.Mode = strPtr(v.Store.Mode.Pretty())
				}
			default:
				inputInfo.Type = "unknown"
				inputInfo.Name = "unknown"
			}

			inputs = append(inputs, inputInfo)
		}
		modInfo.Inputs = inputs

		modules = append(modules, modInfo)
	}
	manifestInfo.Modules = modules

	protoPackageParser, err := NewProtoPackageParser(pkg.ProtoFiles)
	if err != nil {
		return nil, fmt.Errorf("proto package parser: %w", err)
	}
	packageMessageMap, err := protoPackageParser.Parse()
	if err != nil {
		return nil, fmt.Errorf("parse proto files: %w", err)
	}
	manifestInfo.ProtoMessagesByPackage = packageMessageMap

	manifestInfo.ProtoPackages = protoPackageParser.GetPackagesList()
	manifestInfo.ProtoSourceCode = protoPackageParser.GetFilesSourceCode()

	if pkg.SinkConfig != nil {
		desc, files, err := manifest.DescribeSinkConfigs(pkg)
		if err != nil {
			return nil, fmt.Errorf("describe sink configs: %w", err)
		}
		manifestInfo.SinkInfo = &SinkInfo{
			Configs: desc,
			TypeUrl: pkg.SinkConfig.TypeUrl,
			Files:   files,
		}
	}

	return manifestInfo, nil
}

func Extended(manifestPath string, outputModule string, skipValidation bool) (*ExtendedInfo, error) {
	var opts []manifest.Option
	if skipValidation {
		opts = append(opts, manifest.SkipPackageValidationReader())
	}
	reader, err := manifest.NewReader(manifestPath, opts...)
	if err != nil {
		return nil, fmt.Errorf("manifest reader: %w", err)
	}

	pkg, err := reader.Read()
	if err != nil {
		return nil, fmt.Errorf("read manifest %q: %w", manifestPath, err)
	}

	return ExtendedWithPackage(pkg, outputModule)
}

func ExtendedWithPackage(pkg *pbsubstreams.Package, outputModule string) (*ExtendedInfo, error) {
	basicInfo, err := Basic(pkg)
	if err != nil {
		return nil, err
	}

	var stages [][][]string
	if outputModule != "" {
		outputGraph, err := outputmodules.NewOutputModuleGraph(outputModule, true, pkg.Modules)
		if err != nil {
			return nil, fmt.Errorf("creating output module graph: %w", err)
		}
		stages = make([][][]string, 0, len(outputGraph.StagedUsedModules()))
		for _, layers := range outputGraph.StagedUsedModules() {
			var layerDefs [][]string
			for _, l := range layers {
				var mods []string
				for _, m := range l {
					mods = append(mods, m.Name)
				}
				layerDefs = append(layerDefs, mods)
			}
			stages = append(stages, layerDefs)
		}
	}

	return &ExtendedInfo{
		BasicInfo:       basicInfo,
		ExecutionStages: stages,
	}, nil
}

func strPtr(s string) *string {
	return &s
}
