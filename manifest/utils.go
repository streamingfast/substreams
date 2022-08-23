package manifest

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// mapSlice represents a map in the form of a list of key/value pairs (key/value
// pair of `[2]string` where index 0 is the key and index 1 is the value).
type mapSlice [][2]string

func (s *mapSlice) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind != yaml.MappingNode {
		return fmt.Errorf("expected map")
	}

	if len(n.Content)%2 != 0 {
		return fmt.Errorf("invalid map, unequal number of nodes below")
	}

	for i := 0; i < len(n.Content); i += 2 {
		k := n.Content[i].Value
		v := n.Content[i+1].Value
		*s = append(*s, [2]string{k, v})
	}

	return nil
}
