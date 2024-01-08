package main

import (
	_ "github.com/streamingfast/substreams/wasm/bench/substreams_wasi_go/lib"
	"github.com/streamingfast/substreams/wasm/wasi/substreams"
)

func main() {
	substreams.Main()
}
