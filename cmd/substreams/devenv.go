package main

import (
	"github.com/streamingfast/substreams/tools"
	"github.com/streamingfast/substreams/tools/devenv"
)

func init() {
	tools.Cmd.AddCommand(devenv.Cmd)
}
