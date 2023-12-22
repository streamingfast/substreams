package substream

import (
	"fmt"
)

type execFunc func([]byte) error

var moduleRegistry = map[string]execFunc{}

func Register(name string, f execFunc) {
	moduleRegistry[name] = f
}

func Main() {
	//TODO: read args to get module name
	moduleName := "mapBlock" //args[1]

	input, err := ReadInput()
	if err != nil {
		panic(fmt.Errorf("reading input: %w", err))
	}

	execFunc := moduleRegistry[moduleName]
	err = execFunc(input)
	if err != nil {
		panic(fmt.Errorf("executing module %q: %w", moduleName, err))
	}
}
