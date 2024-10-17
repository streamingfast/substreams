package codegen

import (
	"errors"
	"fmt"
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
	if filepath.IsAbs(outputPath) {
		if wd, err := os.Getwd(); err == nil {
			if rel, err := filepath.Rel(wd, outputPath); err == nil {
				outputPath = rel
			}
		}
	}

	return &ProtoGenerator{
		outputPath:    outputPath,
		excludedPaths: excludedPaths,
		generateMod:   generateMod,
	}
}

func (g *ProtoGenerator) GenerateProto(pkg *pbsubstreams.Package) error {
	spkgTemporaryFilePath := filepath.Join(os.TempDir(), pkg.PackageMeta[0].Name+".tmp.spkg")
	cnt, err := proto.Marshal(pkg)
	if err != nil {
		return fmt.Errorf("marshalling package: %w", err)
	}

	if err := os.WriteFile(spkgTemporaryFilePath, cnt, 0644); err != nil {
		return fmt.Errorf("writing %q: %w", spkgTemporaryFilePath, err)
	}

	_, err = os.Stat("buf.gen.yaml")
	bufFileNotFound := errors.Is(err, os.ErrNotExist)
	prostVersion := "v0.4.0"
	prostCrateVersion := "v0.4.1"

	if bufFileNotFound {
		// Beware, the indentation after initial column is important, it's 2 spaces!
		content := dedent.Dedent(`
		    version: v1
		    plugins:
		    - plugin: buf.build/community/neoeinstein-prost:` + prostVersion + `
		      out: ` + g.outputPath + `
		      opt:
		        - file_descriptor_set=false
		`)

		if g.generateMod {
			// Beware, the indentation after initial column is important, it's 2 spaces!
			content += dedent.Dedent(`
				- plugin: buf.build/community/neoeinstein-prost-crate:` + prostCrateVersion + `
				  out: ` + g.outputPath + `
				  opt:
				    - no_features
			`)
		}

		fmt.Printf("Generating 'buf.gen.yaml' for protobuf generation using neoeinstein-prost %q and neoeinstein-prost-crate %q\n", prostVersion, prostCrateVersion)

		if err := os.WriteFile("buf.gen.yaml", []byte(content), 0644); err != nil {
			return fmt.Errorf("error writing buf.gen.yaml: %w", err)
		}
	}

	cmdArgs := []string{
		"generate", spkgTemporaryFilePath + "#format=bin",
	}

	for _, excludePath := range g.excludedPaths {
		cmdArgs = append(cmdArgs, "--exclude-path", excludePath)
	}

	cmdArgs = append(cmdArgs, "--include-imports")

	fmt.Printf("Running: buf %s\n", strings.Join(cmdArgs, " "))
	c := exec.Command("buf", cmdArgs...)
	c.Stdin = os.Stdin
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	if err := c.Run(); err != nil {
		if strings.Contains(err.Error(), "not authenticated") {
			return fmt.Errorf("error executing 'buf':: %w. Make sure that you don't have expired credentials in $HOME/.netrc (You do not need to be authenticated, but you cannot have wrong or expired credentials)", err)
		}
		if strings.Contains(err.Error(), "not found") {
			return fmt.Errorf("error executing 'buf':: %w. Make sure that you have the 'buf' CLI installed: https://buf.build/product/cli", err)

		}
		return fmt.Errorf("error executing 'buf':: %w", err)
	}

	return nil
}
