package server

import (
	"fmt"
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
