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
	// projectPath is the directory holding `buf.gen.yaml` and `Cargo.toml`. It is the
	// manifest's directory, which is not necessarily the working directory.
	projectPath                    string
	generateMod                    bool
	hasNonDeterministicDescriptors bool
	// outputPathWasSet records that the caller chose the output directory, so an existing
	// `buf.gen.yaml` does not override it.
	outputPathWasSet bool
}

func NewProtoGenerator(outputPath string, excludedPaths []string, generateMod bool) *ProtoGenerator {
	return &ProtoGenerator{
		outputPath:    outputPath,
		excludedPaths: excludedPaths,
		generateMod:   generateMod,
	}
}

// SetProjectPath sets the project directory: where `buf.gen.yaml` and `Cargo.toml` are read
// from, where `buf` runs, and what the configuration's `out:` is relative to. It defaults to
// the working directory.
//
// `buf` resolves `buf.gen.yaml` and the paths inside it against its own working directory, so
// every path has to share one base or the configuration written for one run does not mean the
// same thing on the next.
func (g *ProtoGenerator) SetProjectPath(path string) {
	g.projectPath = path

	// Only an absolute output path needs rebasing. A relative one was already given
	// relative to the project, which is what `buf.gen.yaml` needs it to be.
	if !filepath.IsAbs(g.outputPath) {
		return
	}

	abs, err := filepath.Abs(path)
	if err != nil {
		return
	}
	if rel, err := filepath.Rel(abs, g.outputPath); err == nil {
		g.outputPath = rel
	}
}

// SetOutputPathExplicit marks the output directory as the caller's choice, so an existing
// `buf.gen.yaml` does not override it.
func (g *ProtoGenerator) SetOutputPathExplicit() {
	g.outputPathWasSet = true
}

// projectFile resolves name against the project directory.
func (g *ProtoGenerator) projectFile(name string) string {
	return filepath.Join(g.projectPath, name)
}

// bufConfigDir is the directory `buf` runs in, which is the nearest one at or above the project
// holding a `buf.gen.yaml`.
//
// A repository can keep one configuration above several manifests. `buf` resolves both the
// configuration and the `out:` paths inside it against its own working directory, so running
// anywhere else would ignore that configuration and write a second one beside the manifest.
//
// The search stops at the repository root, so a project inside a repository that configures `buf`
// for something else at its root, generating Go say, is not generated with that configuration
// and into its `out:` paths. A stray `buf.gen.yaml` in a home directory is out of reach for the
// same reason.
func (g *ProtoGenerator) bufConfigDir() string {
	current, err := filepath.Abs(g.projectPath)
	if err != nil {
		return g.projectPath
	}

	for {
		if _, err := os.Stat(filepath.Join(current, "buf.gen.yaml")); err == nil {
			return current
		}

		if isRepositoryRoot(current) {
			return g.projectPath
		}

		parent := filepath.Dir(current)
		if parent == current {
			return g.projectPath
		}
		current = parent
	}
}

// isRepositoryRoot reports whether dir holds the marker of a version-control root, which is as
// far up as a configuration can belong to the same project.
func isRepositoryRoot(dir string) bool {
	for _, marker := range []string{".git", ".hg", ".svn"} {
		if _, err := os.Stat(filepath.Join(dir, marker)); err == nil {
			return true
		}
	}
	return false
}

// bufConfigFile is the `buf.gen.yaml` this project generates with, whether or not it exists yet.
func (g *ProtoGenerator) bufConfigFile() string {
	return filepath.Join(g.bufConfigDir(), "buf.gen.yaml")
}

// outputDir is the generated-code directory as this process must open it. `outputPath` is
// relative to the project because it is written into `buf.gen.yaml`, where `buf` resolves it.
func (g *ProtoGenerator) outputDir() string {
	return g.projectFile(g.outputPath)
}

// adoptConfiguredOutputPath points the generator at the directory an existing `buf.gen.yaml`
// generates into, so `mod.rs` lands beside the files it includes.
//
// `buf` writes where the configuration says, which is not always `src/pb`. Looking only in the
// default would leave a project that generates elsewhere with its per-package files written and
// no `mod.rs` tying them together, and nothing said about it. A path given explicitly is the
// caller's choice and is left alone.
func (g *ProtoGenerator) adoptConfiguredOutputPath() {
	if g.outputPathWasSet {
		return
	}

	configured := buffaOutputPath(g.bufConfigFile())
	if configured == "" {
		return
	}

	// `out:` is relative to the directory `buf` runs in, while `outputPath` is relative to the
	// project. They are the same directory only when the configuration sits beside the manifest.
	dir, err := filepath.Abs(g.bufConfigDir())
	if err != nil {
		return
	}
	project, err := filepath.Abs(g.projectPath)
	if err != nil {
		return
	}
	rel, err := filepath.Rel(project, filepath.Join(dir, configured))
	if err != nil {
		return
	}
	g.outputPath = rel
}

// bufOutputPath is the generated-code directory as `buf` must see it: relative to the directory
// `buf` runs in, which is not always the project directory.
func (g *ProtoGenerator) bufOutputPath() string {
	dir, err := filepath.Abs(g.bufConfigDir())
	if err != nil {
		return g.outputPath
	}
	out, err := filepath.Abs(g.outputDir())
	if err != nil {
		return g.outputPath
	}
	rel, err := filepath.Rel(dir, out)
	if err != nil {
		return g.outputPath
	}
	return rel
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

	// Hash what the output was generated for, not only what it was generated from. Repointing
	// `buf.gen.yaml` at another plugin, or changing where it writes, leaves the protos untouched
	// but makes the code on disk wrong for the project.
	for _, plugin := range readBufPlugins(g.bufConfigFile()) {
		hasher.Write([]byte("plugin:" + plugin.Plugin + "|" + plugin.Remote + "|" + plugin.Name + "|" + plugin.Out))
	}

	return hex.EncodeToString(hasher.Sum(nil)), nil
}

// readLastGeneratedHash reads the hash from .last_generated_hash file
func (g *ProtoGenerator) readLastGeneratedHash() (string, error) {
	hashFilePath := filepath.Join(g.outputDir(), ".last_generated_hash")
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
	if err := os.MkdirAll(g.outputDir(), 0755); err != nil {
		return fmt.Errorf("creating output directory: %w", err)
	}

	hashFilePath := filepath.Join(g.outputDir(), ".last_generated_hash")
	if err := os.WriteFile(hashFilePath, []byte(hash), 0644); err != nil {
		return fmt.Errorf("writing hash file: %w", err)
	}
	return nil
}

// hasGeneratedFiles checks if the output directory contains generated files
func (g *ProtoGenerator) hasGeneratedFiles() bool {
	entries, err := os.ReadDir(g.outputDir())
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

	// Without a configuration there is nothing on disk saying what the output on disk was
	// generated for, and the next run writes a new one. Generating is what makes the two agree,
	// so a project whose `buf.gen.yaml` was deleted, which is what the stack-mismatch warning
	// tells the reader to do, must not be skipped as already up to date.
	if _, err := os.Stat(g.bufConfigFile()); err != nil {
		return false, nil
	}

	if !g.generateMod || !generatesBuffaOutput(g.bufConfigFile()) {
		return true, nil
	}

	return g.moduleTreeIsComplete(pkg)
}

// warnOnStackMismatch reports a `buf.gen.yaml` that generates for a different protobuf
// implementation than the one `Cargo.toml` depends on, which does not compile.
func (g *ProtoGenerator) warnOnStackMismatch() {
	config := g.bufConfigFile()
	if _, err := os.Stat(config); err != nil {
		return
	}

	usesBuffa := generatesBuffaOutput(config)
	switch stack := detectProtobufStack(findCargoManifest(g.projectPath)); {
	case stack == stackBuffa && !usesBuffa:
		fmt.Printf("⚠️  Cargo.toml depends on buffa but buf.gen.yaml generates prost bindings\n")
		fmt.Printf("   Point buf.gen.yaml at %s, or delete it to have one generated\n", buffaPlugin)
	case stack == stackProst && usesBuffa:
		fmt.Printf("⚠️  Cargo.toml depends on prost but buf.gen.yaml generates buffa bindings\n")
		fmt.Printf("   Point buf.gen.yaml at %s, or delete it to have one generated\n", prostPlugin)
	}
}

func (g *ProtoGenerator) GenerateProto(pkg *pbsubstreams.Package) error {
	g.adoptConfiguredOutputPath()

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
		g.warnOnStackMismatch()
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

	_, err = os.Stat(g.bufConfigFile())
	bufFileNotFound := errors.Is(err, os.ErrNotExist)
	buffaVersion := "v0.9.2"
	prostVersion := "v0.4.0"
	prostCrateVersion := "v0.4.1"

	// Generating buffa bindings for a project whose code is written against prost does not
	// compile, so buffa is used only where the manifest asks for it. Anything else, including a
	// manifest that names neither and one that cannot be read, keeps the prost output the CLI
	// has always produced.
	generateWithProst := detectProtobufStack(findCargoManifest(g.projectPath)) != stackBuffa

	if bufFileNotFound {
		var content string
		if generateWithProst {
			// Beware, the indentation after initial column is important, it's 2 spaces!
			content = dedent.Dedent(`
			    version: v1
			    plugins:
			    - plugin: ` + prostPlugin + `:` + prostVersion + `
			      out: ` + g.bufOutputPath() + `
			      opt:
			        - file_descriptor_set=false
			`)

			if g.generateMod {
				content += dedent.Dedent(`
					- plugin: ` + prostCratePlugin + `:` + prostCrateVersion + `
					  out: ` + g.bufOutputPath() + `
					  opt:
					    - no_features
				`)
			}
		} else {
			// Beware, the indentation after initial column is important, it's 2 spaces!
			content = dedent.Dedent(`
			    version: v1
			    plugins:
			    - plugin: ` + buffaPlugin + `:` + buffaVersion + `
			      out: ` + g.bufOutputPath() + `
			      opt:
			        - lazy_views=true
			        - unknown_fields=false
			        - idiomatic_field_names=true
			`)
		}

		if err := os.WriteFile(g.bufConfigFile(), []byte(content), 0644); err != nil {
			return fmt.Errorf("error writing buf.gen.yaml: %w", err)
		}
	}

	g.warnOnStackMismatch()

	cmdArgs := []string{
		"generate", spkgTemporaryFilePath + "#format=bin",
	}

	for _, excludePath := range g.excludedPaths {
		cmdArgs = append(cmdArgs, "--exclude-path", excludePath)
	}

	cmdArgs = append(cmdArgs, "--include-imports")

	if bufFileNotFound && generateWithProst {
		fmt.Printf("📋 Generated buf.gen.yaml using neoeinstein-prost %s (Cargo.toml depends on prost)\n", prostVersion)
	} else if bufFileNotFound {
		fmt.Printf("📋 Generated buf.gen.yaml using buffa %s\n", buffaVersion)
	} else {
		fmt.Printf("📋 Using existing buf.gen.yaml configuration\n")
	}

	fmt.Printf("📦 Generating protobuf code \033[90m(buf %s)\033[0m\n", formatBufCommand(cmdArgs))
	c := exec.Command("buf", cmdArgs...)
	c.Dir = g.bufConfigDir()
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
		hashFilePath := filepath.Join(g.outputDir(), ".last_generated_hash")
		os.Remove(hashFilePath) // Ignore errors, file may not exist
	} else {
		// Recompute rather than storing the hash from the top of this function. A run that
		// writes `buf.gen.yaml` itself hashed the plugins before the file existed, which no
		// later run can reproduce, so storing that value would miss the cache every time.
		generatedHash, err := g.calculateHash(pkg)
		if err != nil {
			return fmt.Errorf("calculating hash: %w", err)
		}

		if err := g.writeLastGeneratedHash(generatedHash); err != nil {
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
	if !generatesBuffaOutput(g.bufConfigFile()) {
		return nil
	}

	packages, rootFiles := g.treePackages(pkg)
	if len(packages) == 0 && len(rootFiles) == 0 {
		return nil
	}

	_, err := g.moduleTreeFile().write(renderModuleTree(packages, rootFiles))
	return err
}

// moduleTreeFile is the `mod.rs` this generator owns.
func (g *ProtoGenerator) moduleTreeFile() *generatedFile {
	return newGeneratedFile(filepath.Join(g.outputDir(), "mod.rs"), generatedModHeader)
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
	_, err := os.Stat(filepath.Join(g.outputDir(), name+".mod.rs"))
	return err == nil
}
