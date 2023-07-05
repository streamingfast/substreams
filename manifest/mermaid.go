package manifest

import (
	"bytes"
	"compress/zlib"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

func GenerateMermaidLiveURL(mods *pbsubstreams.Modules) string {
	var buf bytes.Buffer
	w := zlib.NewWriter(&buf)
	var mermaidLive = struct {
		Code          string `json:"code"`
		Mermaid       string `json:"mermaid"`
		AutoSync      bool   `json:"autoSync"`
		UpdateDiagram bool   `json:"updateDiagram"`
	}{}
	mermaidLive.Code = generateMermaidGraph(mods)
	mermaidLive.Mermaid = `{"theme":"default"}`
	mermaidLive.AutoSync = true
	mermaidLive.UpdateDiagram = true
	cnt, _ := json.Marshal(mermaidLive)
	_, _ = w.Write(cnt)
	_ = w.Close()
	b64str := base64.RawURLEncoding.EncodeToString(buf.Bytes())

	return fmt.Sprintf("https://mermaid.live/edit#pako:%s\n", b64str)
}

func PrintMermaid(mods *pbsubstreams.Modules) {
	fmt.Println("Mermaid graph:\n\n```mermaid")
	fmt.Println(generateMermaidGraph(mods))
	fmt.Println("```")
	fmt.Println("")
}

func generateMermaidGraph(mods *pbsubstreams.Modules) string {

	var str strings.Builder
	str.WriteString("graph TD;\n")

	for _, s := range mods.Modules {
		// fmt.Println("module", s.Filename)
		switch s.Kind.(type) {
		case *pbsubstreams.Module_KindMap_:
			str.WriteString(fmt.Sprintf("  %s[map: %s];\n", s.Name, s.Name))
		case *pbsubstreams.Module_KindStore_:
			str.WriteString(fmt.Sprintf("  %s[store: %s];\n", s.Name, s.Name))
		}

		for _, in := range s.Inputs {
			switch input := in.Input.(type) {
			case *pbsubstreams.Module_Input_Source_:
				name := input.Source.Type
				str.WriteString(fmt.Sprintf("  %s[source: %s] --> %s;\n", name, name, s.Name))
			case *pbsubstreams.Module_Input_Map_:
				name := input.Map.ModuleName
				str.WriteString(fmt.Sprintf("  %s --> %s;\n", name, s.Name))
			case *pbsubstreams.Module_Input_Store_:
				name := input.Store.ModuleName
				mode := strings.ToLower(fmt.Sprintf("%s", input.Store.Mode))
				if mode == "deltas" {
					str.WriteString(fmt.Sprintf("  %s -- deltas --> %s;\n", name, s.Name))
				} else {
					str.WriteString(fmt.Sprintf("  %s --> %s;\n", name, s.Name))
				}
			case *pbsubstreams.Module_Input_Params_:
				name := s.Name + ":params"
				str.WriteString(fmt.Sprintf("  %s[params] --> %s;\n", name, s.Name))
			}
		}
	}

	return str.String()
}
