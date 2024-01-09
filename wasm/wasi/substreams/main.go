package substreams

import (
	"flag"
	"fmt"
	"os"
)

type execFunc func([]byte) error

var moduleRegistry = map[string]execFunc{}

func Register(name string, f execFunc) {
	moduleRegistry[name] = f
}

func Main() {
	inputsizeval := flag.String("inputsize", "0", "input size")
	flag.Parse()

	inputsize := 0
	if *inputsizeval != "0" {
		//parse to int
		_, err := fmt.Sscanf(*inputsizeval, "%d", &inputsize)
		if err != nil {
			//ignored.  will use 0 as default
		}
	}

	moduleName := os.Args[0]
	input, err := ReadInput(inputsize)
	if err != nil {
		panic(fmt.Errorf("reading input: %w", err))
	}

	execFunc := moduleRegistry[moduleName]
	err = execFunc(input)
	if err != nil {
		panic(fmt.Errorf("executing module %q: %w", moduleName, err))
	}
}
