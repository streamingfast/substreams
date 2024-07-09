package main

import (
	"encoding/json"
	"fmt"
	"os"
)

//go:generate substreams protogen ./substreams.yaml --with-tinygo-maps // creates genre substreams.gen.go

// Dans WASI: _start
func main() {}

// Log a line to the Substreams engine
func logf(message string, args ...any) {
	fmt.Println(fmt.Sprintf(message, args...))
}

type StoreAddUint64 func(uint64) (string, error)

func PrepareStore(fixtureFile string) StoreAddUint64 {
	return StoreAddUint64(func(code uint64) (string, error) {
		res, _ := os.ReadFile(fixtureFile)
		var fixture []byte
		json.Unmarshal(res, &fixture)
		return string(fixture[int(code)]), nil
	})
}
