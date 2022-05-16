package manifest

import (
	"fmt"
	"strings"
)

func (m *Manifest) PrintMermaid() {
	fmt.Println("Mermaid graph:\n\n```mermaid\ngraph TD;")

	for _, s := range m.Modules {
		for _, in := range s.Inputs {
			if in.Mode != "" && in.Mode == "deltas" {
				fmt.Printf("  %s[%s] -- %q --> %s\n",
					strings.Split(in.Name, ":")[1],
					strings.Replace(in.Name, ":", ": ", 1),
					in.Mode,
					s.Name)
			} else {
				fmt.Printf("  %s[%s] --> %s\n",
					strings.Split(in.Name, ":")[1],
					strings.Replace(in.Name, ":", ": ", 1),
					s.Name)
			}
		}
	}

	fmt.Println("```")
	fmt.Println("")
}
