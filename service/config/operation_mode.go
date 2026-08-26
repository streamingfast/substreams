package config

import (
	"fmt"
	"strings"
)

// OperationMode selects the operating profile of a tier1 server.
type OperationMode string

const (
	// OperationModeDefault is the regular tier1 behavior.
	OperationModeDefault OperationMode = ""

	// OperationModeRollingWindow serves a chain that only keeps a rolling window of
	// blocks: store modules are refused and the first streamable block is treated as a
	// moving lower bound rather than a fixed chain property.
	OperationModeRollingWindow OperationMode = "rolling-window"
)

// ParseOperationMode resolves the value of the operation-mode flag. An empty value
// yields OperationModeDefault.
func ParseOperationMode(in string) (OperationMode, error) {
	switch strings.ToLower(strings.NewReplacer("_", "", "-", "", " ", "").Replace(in)) {
	case "", "default":
		return OperationModeDefault, nil
	case "rollingwindow":
		return OperationModeRollingWindow, nil
	}
	return OperationModeDefault, fmt.Errorf("unknown operation mode %q, valid values are \"default\" and %q", in, OperationModeRollingWindow)
}
