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
	excludedPaths                  []string
	outputPath                     string
	generateMod                    bool
	hasNonDeterministicDescriptors bool
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

// SetHasNonDeterministicDescriptors sets whether the manifest has non-deterministic
// descriptor sets. When true, protobuf generation will always run regardless of
// hash comparison, since the content may have changed without the hash changing.
func (g *ProtoGenerator) SetHasNonDeterministicDescriptors(has bool) {
	g.hasNonDeterministicDescriptors = has
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

// canSkipGeneration reports whether the output on disk already matches the inputs.
func (g *ProtoGenerator) canSkipGeneration(pkg *pbsubstreams.Package, currentHash string) (bool, error) {
	// A non-deterministic descriptor set can change without the hash changing.
	if g.hasNonDeterministicDescriptors {
		return false, nil
	}

	lastHash, err := g.readLastGeneratedHash()
	if err != nil {
		return false, fmt.Errorf("reading last generated hash: %w", err)
	}

	if lastHash == "" || lastHash != currentHash || !g.hasGeneratedFiles() {
		return false, nil
	}

	if !g.generateMod {
		return true, nil
	}

	return g.moduleTreeIsComplete(pkg)
}

func (g *ProtoGenerator) GenerateProto(pkg *pbsubstreams.Package) error {
	// Calculate current hash of inputs
	currentHash, err := g.calculateHash(pkg)
	if err != nil {
		return fmt.Errorf("calculating hash: %w", err)
	}

	skip, err := g.canSkipGeneration(pkg, currentHash)
	if err != nil {
		return err
	}
	if skip {
		// Nothing else writes mod.rs on this path.
		if g.generateMod {
			if err := g.writeModuleTree(pkg); err != nil {
				return fmt.Errorf("writing module tree: %w", err)
			}
		}
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
	buffaVersion := "v0.9.2"

	if bufFileNotFound {
		// Beware, the indentation after initial column is important, it's 2 spaces!
		content := dedent.Dedent(`
		    version: v1
		    plugins:
		    - plugin: buf.build/anthropics/buffa:` + buffaVersion + `
		      out: ` + g.outputPath + `
		      opt:
		        - lazy_views=true
		        - unknown_fields=false
		        - idiomatic_field_names=true
		`)

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
		fmt.Printf("📋 Generated buf.gen.yaml using buffa %s\n", buffaVersion)
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

	// Update hash file after successful generation (only if all descriptor sets are deterministic)
	if g.hasNonDeterministicDescriptors {
		// Remove the hash file if it exists, since we can't reliably cache
		// when descriptor sets may change without version changes
		hashFilePath := filepath.Join(g.outputPath, ".last_generated_hash")
		os.Remove(hashFilePath) // Ignore errors, file may not exist
	} else {
		if err := g.writeLastGeneratedHash(currentHash); err != nil {
			return fmt.Errorf("writing hash file: %w", err)
		}
	}

	if g.generateMod {
		if err := g.writeModuleTree(pkg); err != nil {
			return fmt.Errorf("writing module tree: %w", err)
		}
	}

	fmt.Printf("🎯 Protobuf generation complete\n")
	return nil
}

// writeModuleTree writes the `mod.rs` wiring buffa's per-package files into the crate.
//
// Every package in the descriptor set is wired up, transitive imports included: buffa
// refers to a type in another package through a relative `super::...::other_pkg` path,
// so leaving a package out breaks the packages that reference it. A descriptor with no
// `package` sits at the root, where buffa's own codegen puts it.
//
// A `mod.rs` this function did not write is left untouched: it may be hand-written or
// hand-edited, and silently replacing it would produce a spurious diff.
func (g *ProtoGenerator) writeModuleTree(pkg *pbsubstreams.Package) error {
	packages, rootFiles := g.treePackages(pkg)
	if len(packages) == 0 && len(rootFiles) == 0 {
		return nil
	}

	_, err := g.moduleTreeFile().write(renderModuleTree(packages, rootFiles))
	return err
}

// moduleTreeFile is the `mod.rs` this generator owns.
func (g *ProtoGenerator) moduleTreeFile() *generatedFile {
	return newGeneratedFile(filepath.Join(g.outputPath, "mod.rs"), generatedModHeader)
}

// moduleTreeIsComplete reports whether the packages buf generated files for are wired
// up in the `mod.rs` currently on disk. A `mod.rs` belonging to someone else is their
// business, so it counts as complete.
func (g *ProtoGenerator) moduleTreeIsComplete(pkg *pbsubstreams.Package) (bool, error) {
	owner, err := g.moduleTreeFile().ownership()
	if err != nil {
		return false, err
	}
	if owner == ownershipForeign {
		return true, nil
	}

	// treePackages keeps only the packages whose generated file is present, so an
	// empty tree for a descriptor that declares packages means the output is gone.
	// A missing mod.rs is not itself a reason to rerun buf, since writeModuleTree
	// restores it from the files already on disk.
	packages, rootFiles := g.treePackages(pkg)
	return len(packages) > 0 || len(rootFiles) > 0 || len(pkg.GetProtoFiles()) == 0, nil
}

// treePackages returns the proto packages to wire up, plus the generated files for
// descriptors that declare no package (buffa writes those to `__buffa.mod.rs` and
// expects them at the root of the tree).
func (g *ProtoGenerator) treePackages(pkg *pbsubstreams.Package) (packages []string, rootFiles []string) {
	seen := map[string]bool{}

	for _, protoFile := range pkg.GetProtoFiles() {
		// buffa writes a descriptor with no `package` to `__buffa.mod.rs`.
		name := protoFile.GetPackage()
		if name == "" {
			name = rootModuleFile
		}
		if seen[name] {
			continue
		}

		if !g.hasModuleFile(name) {
			continue
		}

		seen[name] = true
		if name == rootModuleFile {
			rootFiles = append(rootFiles, name)
		} else {
			packages = append(packages, name)
		}
	}

	sort.Strings(packages)
	sort.Strings(rootFiles)
	return packages, rootFiles
}

// hasModuleFile reports whether buffa emitted a module file for this package. A
// descriptor can be present without generated output, such as an excluded path or one
// the plugin skipped, and wiring up a missing file would not compile.
func (g *ProtoGenerator) hasModuleFile(name string) bool {
	_, err := os.Stat(filepath.Join(g.outputPath, name+".mod.rs"))
	return err == nil
}
