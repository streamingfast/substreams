package test

import (
	"bufio"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"github.com/itchyny/gojq"
	"github.com/streamingfast/substreams/tools/test/comparator"
	"gopkg.in/yaml.v3"
	"io"
	"os"
	"path/filepath"
	"strconv"
)

type Spec struct {
	Tests []*TestConfig `json:"tests"`
}

type TestConfig struct {
	Module string `json:"module" yaml:"module"`
	Block  uint64 `json:"block" yaml:"block"`
	Path   string `json:"path" yaml:"path"`
	Expect string `json:"expect" yaml:"expect"`
	Op     string `json:"op,omitempty" yaml:"op"`
	Args   string `json:"args,omitempty" yaml:"args"`
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

	ext := filepath.Ext(path)
	switch ext {

	case ".jsonl":
		return readSpecFromJSONL(file)
	case ".csv":
		return readSpecFromCSV(file)
	case ".yaml":
		return readSpecFromYAML(file)
	default:
		return nil, fmt.Errorf("unsupported test file type %q", ext)
	}
}

func readSpecFromCSV(file io.Reader) (*Spec, error) {
	csvReader := csv.NewReader(file)
	csvReader.LazyQuotes = true
	records, err := csvReader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("unable to parse csv: %w", err)
	}

	spec := &Spec{
		Tests: nil,
	}
	for idx, line := range records {
		config, err := parseCSVLine(line)
		if err != nil {
			return nil, fmt.Errorf("unable parse line %d: %w", idx, err)
		}
		spec.Tests = append(spec.Tests, config)
	}
	return spec, nil
}

func parseCSVLine(line []string) (*TestConfig, error) {
	if len(line) < 4 {
		return nil, fmt.Errorf("must have at-least 4 values")
	}

	blockNum, err := strconv.ParseUint(line[1], 10, 64)
	if err != nil {
		return nil, fmt.Errorf("parse block num %q: %w", line[1], err)
	}

	config := &TestConfig{
		Module: line[0],
		Block:  blockNum,
		Path:   line[2],
		Expect: line[3],
	}
	if len(line) >= 5 {
		config.Op = line[4]
	}
	if len(line) >= 6 {
		config.Op = line[5]
	}
	return config, nil
}
func readSpecFromJSONL(file io.Reader) (*Spec, error) {
	spec := &Spec{
		Tests: nil,
	}
	reader := bufio.NewReader(file)
	for {
		line, _, err := reader.ReadLine()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("unable to read line: %w", err)
		}

		config := &TestConfig{}
		err = json.Unmarshal(line, config)
		if err != nil {
			return nil, fmt.Errorf("unable unmarshal open object: %w", err)
		}
		spec.Tests = append(spec.Tests, config)
	}
	return spec, nil
}

func readSpecFromYAML(reader io.Reader) (*Spec, error) {
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
