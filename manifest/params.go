package manifest

import (
	"fmt"
	"strings"

	"github.com/schollz/closestmatch"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

func ParseParams(paramsString []string) (map[string]string, error) {
	params := make(map[string]string)
	for _, param := range paramsString {
		parts := strings.SplitN(param, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf(`param %q invalid, must be of the format: "module=value" or "imported:module=value"`, param)
		}
		params[parts[0]] = parts[1]
	}
	return params, nil
}

func ApplyParams(params map[string]string, pkg *pbsubstreams.Package) error {
	for k, v := range params {
		var found bool
		var closest []string
		for _, mod := range pkg.Modules.Modules {
			closest = append(closest, mod.Name)
			if mod.Name == k {
				if len(mod.Inputs) == 0 {
					return fmt.Errorf("param for module %q: missing 'params' module input", mod.Name)
				}
				p := mod.Inputs[0].GetParams()
				if p == nil {
					return fmt.Errorf("param for module %q: first module input is not 'params'", mod.Name)
				}
				p.Value = v
				found = true
			}
		}
		if !found {
			closeEnough := closestmatch.New(closest, []int{2}).Closest(k)
			return fmt.Errorf("param for module %q: module not found, did you mean %q ?", k, closeEnough)
		}
	}
	return nil
}
