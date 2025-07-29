package codegen

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
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

// calculateHash computes a deterministic hash of the proto generation inputs
func (g *ProtoGenerator) calculateHash(pkg *pbsubstreams.Package) (string, error) {
	hasher := sha256.New()
	
	// Hash proto files in a deterministic order
	protoHashes := make([]string, 0, len(pkg.ProtoFiles))
	for _, protoFile := range pkg.ProtoFiles {
		protoBytes, err := proto.Marshal(protoFile)
		if err != nil {
			return "", fmt.Errorf("marshalling proto file: %w", err)
		}
		protoHasher := sha256.New()
		protoHasher.Write(protoBytes)
		protoHashes = append(protoHashes, hex.EncodeToString(protoHasher.Sum(nil)))
	}
	sort.Strings(protoHashes)
	
	// Write proto file hashes
	for _, hash := range protoHashes {
		hasher.Write([]byte(hash))
	}
	
	// Hash excluded paths in deterministic order
	excludedPaths := make([]string, len(g.excludedPaths))
	copy(excludedPaths, g.excludedPaths)
	sort.Strings(excludedPaths)
	for _, path := range excludedPaths {
		hasher.Write([]byte(path))
	}
	
	// Hash generateMod flag
	if g.generateMod {
		hasher.Write([]byte("generateMod:true"))
	} else {
		hasher.Write([]byte("generateMod:false"))
	}
	
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

// readLastGeneratedHash reads the hash from .last_generated_hash file
func (g *ProtoGenerator) readLastGeneratedHash() (string, error) {
	hashFilePath := filepath.Join(g.outputPath, ".last_generated_hash")
	content, err := os.ReadFile(hashFilePath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil // File doesn't exist, return empty hash
		}
		return "", fmt.Errorf("reading hash file: %w", err)
	}
	return strings.TrimSpace(string(content)), nil
}

// writeLastGeneratedHash writes the hash to .last_generated_hash file
func (g *ProtoGenerator) writeLastGeneratedHash(hash string) error {
	// Ensure output directory exists
	if err := os.MkdirAll(g.outputPath, 0755); err != nil {
		return fmt.Errorf("creating output directory: %w", err)
	}
	
	hashFilePath := filepath.Join(g.outputPath, ".last_generated_hash")
	if err := os.WriteFile(hashFilePath, []byte(hash), 0644); err != nil {
		return fmt.Errorf("writing hash file: %w", err)
	}
	return nil
}

// hasGeneratedFiles checks if the output directory contains generated files
func (g *ProtoGenerator) hasGeneratedFiles() bool {
	entries, err := os.ReadDir(g.outputPath)
	if err != nil {
		return false
	}
	
	// Look for .rs files (generated Rust files)
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".rs") {
			return true
		}
	}
	return false
}

func formatBufCommand(cmdArgs []string) string {
	// Make the command more readable by breaking long paths and formatting nicely
	result := make([]string, 0, len(cmdArgs))
	for i, arg := range cmdArgs {
		if i == 0 {
			// First argument is the command (generate)
			result = append(result, arg)
		} else if strings.HasPrefix(arg, "/") && strings.Contains(arg, "tmp") {
			// Format long temporary file paths
			if strings.Contains(arg, "#format=bin") {
				parts := strings.Split(arg, "/")
				if len(parts) > 0 {
					fileName := parts[len(parts)-1]
					result = append(result, fmt.Sprintf("<%s>", fileName))
				} else {
					result = append(result, arg)
				}
			} else {
				result = append(result, arg)
			}
		} else {
			result = append(result, arg)
		}
	}
	return strings.Join(result, " ")
}

func (g *ProtoGenerator) GenerateProto(pkg *pbsubstreams.Package) error {
	// Calculate current hash of inputs
	currentHash, err := g.calculateHash(pkg)
	if err != nil {
		return fmt.Errorf("calculating hash: %w", err)
	}
	
	// Read last generated hash
	lastHash, err := g.readLastGeneratedHash()
	if err != nil {
		return fmt.Errorf("reading last generated hash: %w", err)
	}
	
	// Check if we can skip generation
	if lastHash != "" && lastHash == currentHash && g.hasGeneratedFiles() {
		fmt.Printf("⚡ Protobuf generation skipped (no changes detected)\n")
		return nil
	}

	tmpDir, err := os.MkdirTemp("", "substreams_protogen")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	spkgTemporaryFilePath := filepath.Join(tmpDir, pkg.PackageMeta[0].Name+".tmp.spkg")
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

	if bufFileNotFound {
		fmt.Printf("📋 Generated buf.gen.yaml using neoeinstein-prost %s and neoeinstein-prost-crate %s\n", prostVersion, prostCrateVersion)
	} else {
		fmt.Printf("📋 Using existing buf.gen.yaml configuration\n")
	}
	
	fmt.Printf("📦 Generating protobuf code \033[90m(buf %s)\033[0m\n", formatBufCommand(cmdArgs))
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

	// Update hash file after successful generation
	if err := g.writeLastGeneratedHash(currentHash); err != nil {
		return fmt.Errorf("writing hash file: %w", err)
	}

	fmt.Printf("🎯 Protobuf generation complete\n")
	return nil
}
