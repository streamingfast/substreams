package server

import (
	"fmt"
	"strings"
)

type DeployStatusPrinter struct {
}

func (d *DeployStatusPrinter) Printf(format string, args ...interface{}) {
	fmt.Printf(format, args...)
}

type nilDeployStatusPrinter struct {
}

func (d *nilDeployStatusPrinter) Printf(format string, args ...interface{}) {
	return
}

var defaultDeployStatusPrinter = &nilDeployStatusPrinter{}

func IsTruthy(str string) bool {
	switch strings.ToLower(strings.TrimSpace(str)) {
	case "true", "1", "yes", "on", "y", "enabled":
		return true
	default:
		return false
	}
}
