package manifest

import (
	"bytes"
	"crypto/sha1"
	"encoding/binary"
	"encoding/hex"
	"fmt"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

type ModuleHashes struct {
	cache map[string][]byte
}

func NewModuleHashes() *ModuleHashes {
	return &ModuleHashes{
		cache: make(map[string][]byte),
	}
}

type ModuleHash []byte

func (m *ModuleHashes) Get(moduleName string) string {
	return hex.EncodeToString(m.cache[moduleName])
}

func (m *ModuleHashes) HashModule(modules *pbsubstreams.Modules, module *pbsubstreams.Module, graph *ModuleGraph) ModuleHash {
	if cachedHash := m.cache[module.Name]; cachedHash != nil {
		return cachedHash
	}

	buf := bytes.NewBuffer(nil)

	initialBlockBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(initialBlockBytes, module.InitialBlock) //at this
	// point start block should have been resolved
	buf.WriteString("initial_block")
	buf.Write(initialBlockBytes)

	buf.WriteString("kind")
	switch module.Kind.(type) {
	case *pbsubstreams.Module_KindMap_:
		buf.WriteString("map")
	case *pbsubstreams.Module_KindStore_:
		buf.WriteString("store")
	default:
		panic(fmt.Sprintf("invalid module file %T", module.Kind))
	}

	buf.WriteString("binary")
	buf.WriteString(modules.Binaries[module.BinaryIndex].Type)
	buf.Write(modules.Binaries[module.BinaryIndex].Content)

	buf.WriteString("inputs")
	for _, input := range module.Inputs {
		buf.WriteString(inputName(input))
		buf.WriteString(inputValue(input))
	}

	buf.WriteString("ancestors")
	ancestors, _ := graph.AncestorsOf(module.Name)
	for _, ancestor := range ancestors {
		sig := m.HashModule(modules, ancestor, graph)
		buf.Write(sig)
	}

	buf.WriteString("entrypoint")
	buf.WriteString(module.Name)

	h := sha1.New()
	h.Write(buf.Bytes())

	output := h.Sum(nil)
	m.cache[module.Name] = output
	return output
}

func inputName(input *pbsubstreams.Module_Input) string {
	switch input.Input.(type) {
	case *pbsubstreams.Module_Input_Store_:
		return "store"
	case *pbsubstreams.Module_Input_Source_:
		return "source"
	case *pbsubstreams.Module_Input_Map_:
		return "map"
	default:
		panic(fmt.Sprintf("invalid input %T", input.Input))
	}
}

func inputValue(input *pbsubstreams.Module_Input) string {
	switch input.Input.(type) {
	case *pbsubstreams.Module_Input_Store_:
		return input.GetStore().ModuleName
	case *pbsubstreams.Module_Input_Source_:
		return input.GetSource().Type
	case *pbsubstreams.Module_Input_Map_:
		return input.GetMap().ModuleName
	default:
		panic(fmt.Sprintf("invalid input %T", input.Input))
	}
}
