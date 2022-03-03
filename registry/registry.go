package registry

import (
	"reflect"

	imports "github.com/streamingfast/substreams/native-imports"
)

type FactoryFunc func(imp *imports.Imports) reflect.Value

var registry = map[string]FactoryFunc{}

func Register(name string, f FactoryFunc) {
	registry[name] = f
}

func Init(imp *imports.Imports) map[string]reflect.Value {
	out := make(map[string]reflect.Value)
	for name, f := range registry {
		out[name] = f(imp)
	}
	return out
}
