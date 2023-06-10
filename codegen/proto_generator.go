package codegen

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/lithammer/dedent"
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

func (g *ProtoGenerator) GenerateProto(pkg *pbsubstreams.Package, showBufContent bool) error {
	spkgTemporaryFilePath := filepath.Join(os.TempDir(), pkg.PackageMeta[0].Name+".tmp.spkg")
	cnt, err := proto.Marshal(pkg)
	if err != nil {
		return fmt.Errorf("marshalling package: %w", err)
	}

	if err := os.WriteFile(spkgTemporaryFilePath, cnt, 0644); err != nil {
		fmt.Println("")
		return fmt.Errorf("writing %q: %w", spkgTemporaryFilePath, err)
	}

	_, err = os.Stat("buf.gen.yaml")
	bufFileNotFound := errors.Is(err, os.ErrNotExist)

	if bufFileNotFound {
		// Beware, the indentation after initial column is important, it's 2 spaces!
		content := dedent.Dedent(`
		    version: v1
		    plugins:
		    - plugin: buf.build/community/neoeinstein-prost:v0.2.2
		      out: ` + g.outputPath + `
		      opt:
		        - file_descriptor_set=false
		`)

		if g.generateMod {
			// Beware, the indentation after initial column is important, it's 2 spaces!
			content += dedent.Dedent(`
				- remote: buf.build/prost/plugins/crate:v0.3.1-1
				  out: ` + g.outputPath + `
				  opt:
				    - no_features
			`)
		}

		if showBufContent {
			fmt.Println("Writing to temporary 'buf.gen.yaml'")
			fmt.Println("---")
			fmt.Println(content)
			fmt.Println("---")
		}

		if err := ioutil.WriteFile("buf.gen.yaml", []byte(content), 0644); err != nil {
			return fmt.Errorf("error writing buf.gen.yaml: %w", err)
		}
		defer func() {
			// Too bad if there is an error, nothing we can do
			_ = os.Remove("buf.gen.yaml")
		}()
	}

	cmdArgs := []string{
		"generate", spkgTemporaryFilePath + "#format=bin",
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

	return nil
}
