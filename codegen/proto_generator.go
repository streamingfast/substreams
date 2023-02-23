package codegen

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"

	"google.golang.org/protobuf/proto"
)

type ProtoGenerator struct {
	excludedPaths []string
	outputPath    string
	generateMod   bool
}

func NewProtoGenerator(outputPath string, excludedPaths []string, generateMod bool) *ProtoGenerator {
	return &ProtoGenerator{
		outputPath:    outputPath,
		excludedPaths: excludedPaths,
		generateMod:   generateMod,
	}
}

func (g *ProtoGenerator) GenerateProto(pkg *pbsubstreams.Package) error {

	defaultFilename := filepath.Join(os.TempDir(), "tmp.spkg")
	cnt, err := proto.Marshal(pkg)
	if err != nil {
		return fmt.Errorf("marshalling package: %w", err)
	}

	if err := os.WriteFile(defaultFilename, cnt, 0644); err != nil {
		fmt.Println("")
		return fmt.Errorf("writing %q: %w", defaultFilename, err)
	}

	_, err = os.Stat("buf.gen.yaml")
	bufFileNotFound := errors.Is(err, os.ErrNotExist)

	if bufFileNotFound {
		content := `
version: v1
plugins:
  - remote: buf.build/prost/plugins/prost:v0.1.3-2
    out: ` + g.outputPath + `
    opt:
`
		if g.generateMod {
			content += `
  - remote: buf.build/prost/plugins/crate:v0.3.1-1
    out: ` + g.outputPath + `
    opt:
    - no_features
`
		}
		fmt.Println(`Writing to temporary 'buf.gen.yaml':
---
` + content + `
---`)
		if err := ioutil.WriteFile("buf.gen.yaml", []byte(content), 0644); err != nil {
			return fmt.Errorf("error writing buf.gen.yaml: %w", err)
		}
	}

	spkgFilepath := filepath.Join(os.TempDir(), "tmp.spkg#format=bin")
	cmdArgs := []string{
		"generate", spkgFilepath,
	}
	for _, excludePath := range g.excludedPaths {
		cmdArgs = append(cmdArgs, "--exclude-path", excludePath)
	}
	fmt.Printf("Running: buf %s\n", strings.Join(cmdArgs, " "))
	c := exec.Command("buf", cmdArgs...)
	c.Stdin = os.Stdin
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	if err := c.Run(); err != nil {
		return fmt.Errorf("error executing 'buf':: %w", err)
	}

	if bufFileNotFound {
		fmt.Println("Removing temporary 'buf.gen.yaml'")
		if err := os.Remove("buf.gen.yaml"); err != nil {
			return fmt.Errorf("error deleting buf.gen.yaml: %w", err)
		}
	}
	return nil
}
