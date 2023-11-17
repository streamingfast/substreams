package docker

import "strings"

func isTruthy(str string) bool {
	switch strings.ToLower(strings.TrimSpace(str)) {
	case "true", "1", "yes", "on", "y", "enabled":
		return true
	default:
		return false
	}
}
