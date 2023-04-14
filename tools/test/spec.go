package test

import (
	"fmt"
	"github.com/itchyny/gojq"
	"github.com/streamingfast/substreams/tools/test/comparator"
	"gopkg.in/yaml.v3"
	"io"
	"os"
)

type Spec struct {
	Tests []*TestConfig `json:"tests"`
}

type TestConfig struct {
	Module string `json:"module" yaml:"module"`
	Block  uint64 `json:"block" yaml:"block"`
	Path   string `json:"path" yaml:"path"`
	Expect string `json:"expect" yaml:"expect"`
	Op     string `json:"op;omitempty" yaml:"op"`
	Args   string `json:"args;omitempty" yaml:"args"`
}

func (t *TestConfig) Test(idx int) (*Test, error) {
	query, err := gojq.Parse(t.Path)
	if err != nil {
		return nil, fmt.Errorf("failed to parse jq path: %w", err)
	}
	code, err := gojq.Compile(query)
	if err != nil {
		return nil, fmt.Errorf("failed to compile jq path: %w", err)
	}

	cmp, err := comparator.NewComparable(t.Expect, t.Op, t.Args)
	if err != nil {
		return nil, fmt.Errorf("failed to setup comparator: %w", err)
	}

	return &Test{
		code:       code,
		path:       t.Path,
		block:      t.Block,
		moduleName: t.Module,
		fileIndex:  idx,
		comparable: cmp,
	}, nil

}

func readSpecFromFile(path string) (*Spec, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open: %w", err)
	}
	defer file.Close()

	return readSpecFromReader(file)
}

func readSpecFromReader(reader io.Reader) (*Spec, error) {
	var spec *Spec
	if err := yaml.NewDecoder(reader).Decode(&spec); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}

	return spec, nil
}

type Test struct {
	code       *gojq.Code
	path       string
	comparable comparator.Comparable
	moduleName string
	block      uint64
	fileIndex  int
}

type Result struct {
	test  *Test
	Valid bool
	Msg   string
}
