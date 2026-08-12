//go:build cgo

// The devenv command boots tier1 and tier2 in-process, which pulls in the wasmtime runtime
// (app/tier1.go imports it for its side effect). Those Go bindings are cgo-only, so a
// CGO_ENABLED=0 build cannot link them — and the released Docker image is built exactly that
// way, to get a static binary.
//
// Gating on cgo rather than on a custom tag keeps the command available wherever it is useful
// without asking for a special incantation: a local `go build` or `go install` has cgo on by
// default and gets `substreams tools devenv`. The static image loses it, which costs nothing —
// the command needs a Docker daemon that image does not have.

package main

import (
	"github.com/streamingfast/substreams/tools"
	"github.com/streamingfast/substreams/tools/devenv"
)

func init() {
	tools.Cmd.AddCommand(devenv.Cmd)
}
