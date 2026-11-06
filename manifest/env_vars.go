package manifest

import (
	"fmt"
	"os"
	"strings"
)

// expandEnvVars expands `$VAR` / `${VAR}` references found in `in` using the
// process environment. Unlike os.ExpandEnv, it errors out when a referenced
// variable is not set, instead of silently substituting an empty string. A
// variable that is set but empty is considered valid and expands to "".
//
// When `in` contains no variable reference, it is returned untouched.
func expandEnvVars(in string) (string, error) {
	var missing []string
	out := os.Expand(in, func(name string) string {
		if v, ok := os.LookupEnv(name); ok {
			return v
		}
		missing = append(missing, name)
		return ""
	})

	if len(missing) > 0 {
		return "", fmt.Errorf("unknown environment variable(s) referenced: %s", strings.Join(missing, ", "))
	}

	return out, nil
}
