package manifest

import (
	"bytes"
	"crypto/sha1"
	"encoding/binary"
	"fmt"

	pbtransform "github.com/streamingfast/substreams/pb/sf/substreams/transform/v1"
)

type ModuleSignature []byte

func SignModule(manifest *pbtransform.Manifest, module *pbtransform.Module, graph *ModuleGraph) ModuleSignature {
	buf := bytes.NewBuffer(nil)

	startBlockBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(startBlockBytes, *module.StartBlock) //at this point start block should have been resolved
	buf.WriteString("start_block")
	buf.Write(startBlockBytes)

	buf.WriteString("kind")
	switch module.Kind.(type) {
	case *pbtransform.Module_KindMap:
		buf.WriteString("map")
	case *pbtransform.Module_KindStore:
		buf.WriteString("store")
	default:
		panic(fmt.Sprintf("invalid module file %T", module.Kind))
	}

	buf.WriteString("code")
	switch m := module.Code.(type) {
	case *pbtransform.Module_WasmCode:
		code := manifest.ModulesCode[m.WasmCode.Index]
		buf.Write(code)
		buf.WriteString(m.WasmCode.Entrypoint)
	}

	buf.WriteString("inputs")
	for _, input := range module.Inputs {
		buf.WriteString(inputName(input))
	}

	buf.WriteString("ancestors")
	ancestors, _ := graph.AncestorsOf(module.Name)
	for _, ancestor := range ancestors {
		sig := SignModule(manifest, ancestor, graph)
		buf.Write(sig)
	}

	h := sha1.New()
	h.Write(buf.Bytes())

	return h.Sum(nil)
}

func inputName(input *pbtransform.Input) string {
	switch input.Input.(type) {
	case *pbtransform.Input_Store:
		return "store"
	case *pbtransform.Input_Source:
		return "source"
	case *pbtransform.Input_Map:
		return "map"
	default:
		panic(fmt.Sprintf("invalid input %T", input.Input))
	}
}
