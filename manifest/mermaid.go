package manifest

import (
	"fmt"
	"strings"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

func PrintMermaid(mods *pbsubstreams.Modules) {
	fmt.Println("Mermaid graph:\n\n```mermaid\ngraph TD;")

	for _, s := range mods.Modules {
		// fmt.Println("module", s.Filename)
		switch s.Kind.(type) {
		case *pbsubstreams.Module_KindMap_:
			fmt.Printf("  %s[map: %s]\n", s.Name, s.Name)
		case *pbsubstreams.Module_KindStore_:
			fmt.Printf("  %s[store: %s]\n", s.Name, s.Name)
		}

		for _, in := range s.Inputs {
			switch input := in.Input.(type) {
			case *pbsubstreams.Module_Input_Source_:
				name := input.Source.Type
				fmt.Printf("  %s[source: %s] --> %s\n", name, name, s.Name)
			case *pbsubstreams.Module_Input_Map_:
				name := input.Map.ModuleName
				fmt.Printf("  %s --> %s\n", name, s.Name)
			case *pbsubstreams.Module_Input_Store_:
				name := input.Store.ModuleName
				mode := strings.ToLower(fmt.Sprintf("%s", input.Store.Mode))
				if mode == "deltas" {
					fmt.Printf("  %s -- deltas --> %s\n", name, s.Name)
				} else {
					fmt.Printf("  %s --> %s\n", name, s.Name)
				}
			case *pbsubstreams.Module_Input_Params_:
				name := s.Name + ":params"
				fmt.Printf("  %s[params] --> %s\n", name, s.Name)
			}
		}
	}

	fmt.Println("```")
	fmt.Println("")
}
