package manifest

import (
	"bytes"
	"crypto/sha1"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"sync"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

type ModuleHash []byte

type ModuleHashes struct {
	cache map[string][]byte

	mu *sync.RWMutex
}

func NewModuleHashes() *ModuleHashes {
	return &ModuleHashes{
		cache: make(map[string][]byte),
		mu:    &sync.RWMutex{},
	}
}

func (m *ModuleHashes) Get(moduleName string) string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return hex.EncodeToString(m.cache[moduleName])
}

func (m *ModuleHashes) Iter(cb func(hash, name string) error) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for name, hash := range m.cache {
		if err := cb(hex.EncodeToString(hash), name); err != nil {
			return err
		}
	}
	return nil
}

func (m *ModuleHashes) HashModule(modules *pbsubstreams.Modules, module *pbsubstreams.Module, graph *ModuleGraph) (ModuleHash, error) {
	m.mu.RLock()
	if cachedHash := m.cache[module.Name]; cachedHash != nil {
		m.mu.RUnlock()
		return cachedHash, nil
	}
	m.mu.RUnlock()

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
		return nil, fmt.Errorf("invalid module file %T", module.Kind)
	}

	buf.WriteString("binary")
	buf.WriteString(modules.Binaries[module.BinaryIndex].Type)
	buf.Write(modules.Binaries[module.BinaryIndex].Content)

	buf.WriteString("inputs")
	for _, input := range module.Inputs {
		name, err := inputName(input)
		if err != nil {
			return nil, err
		}
		buf.WriteString(name)

		value, err := inputValue(input)
		if err != nil {
			return nil, err
		}
		buf.WriteString(value)
	}

	buf.WriteString("ancestors")
	ancestors, _ := graph.AncestorsOf(module.Name)
	for _, ancestor := range ancestors {
		sig, err := m.HashModule(modules, ancestor, graph)
		if err != nil {
			return nil, err
		}
		buf.Write(sig)
	}

	buf.WriteString("entrypoint")
	buf.WriteString(module.BinaryEntrypoint)

	h := sha1.New()
	h.Write(buf.Bytes())

	output := h.Sum(nil)
	m.mu.Lock()
	m.cache[module.Name] = output
	m.mu.Unlock()
	return output, nil
}

func inputName(input *pbsubstreams.Module_Input) (string, error) {
	switch input.Input.(type) {
	case *pbsubstreams.Module_Input_Store_:
		return "store", nil
	case *pbsubstreams.Module_Input_Source_:
		return "source", nil
	case *pbsubstreams.Module_Input_Map_:
		return "map", nil
	case *pbsubstreams.Module_Input_Params_:
		return "params", nil
	default:
		return "", fmt.Errorf("invalid input %T", input.Input)
	}
}

func inputValue(input *pbsubstreams.Module_Input) (string, error) {
	switch input.Input.(type) {
	case *pbsubstreams.Module_Input_Source_:
		return input.GetSource().Type, nil
	case *pbsubstreams.Module_Input_Params_:
		return input.GetParams().Value, nil
	case *pbsubstreams.Module_Input_Store_:
		return "", nil // this is accounted for in the `AncestorOf()` tree
	case *pbsubstreams.Module_Input_Map_:
		return "", nil // this is accounted for in the `AncestorOf()` tree
	default:
		return "", fmt.Errorf("invalid input %T", input.Input)
	}
}
