package manifest

import (
	"fmt"
	"strings"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

func PrintMermaid(mods *pbsubstreams.Modules) {
	fmt.Println("Mermaid graph:\n\n```mermaid\ngraph TD;")

	for _, s := range mods.Modules {
		for _, in := range s.Inputs {
			var name string
			var mode string
			switch input := in.Input.(type) {
			case *pbsubstreams.Module_Input_Source_:
				name = input.Source.Type
			case *pbsubstreams.Module_Input_Map_:
				name = input.Map.ModuleName
			case *pbsubstreams.Module_Input_Store_:
				name = input.Store.ModuleName
				mode = strings.ToLower(fmt.Sprintf("%s", input.Store.Mode))
			}
			if mode != "" && mode == "deltas" {
				fmt.Printf("  %s[%s] -- %q --> %s\n",
					strings.Split(name, ":")[1],
					strings.Replace(name, ":", ": ", 1),
					mode,
					s.Name)
			} else {
				fmt.Printf("  %s[%s] --> %s\n",
					strings.Split(name, ":")[1],
					strings.Replace(name, ":", ": ", 1),
					s.Name)
			}
		}
	}

	fmt.Println("```")
	fmt.Println("")
}
