package main

import (
	"github.com/streamingfast/substreams-sdk-go/substreams"
	_ "github.com/streamingfast/substreams/wasm/bench/substreams_wasi_go/lib"
)

func main() {
	substreams.Main()
}
