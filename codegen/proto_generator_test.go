package codegen

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"google.golang.org/protobuf/types/descriptorpb"
)

func TestCalculateHash(t *testing.T) {
	// Create a test proto file
	testProtoFile := &descriptorpb.FileDescriptorProto{
		Name:    stringPtr("test.proto"),
		Package: stringPtr("test"),
	}

	pkg := &pbsubstreams.Package{
		ProtoFiles: []*descriptorpb.FileDescriptorProto{testProtoFile},
	}

	generator := NewProtoGenerator("test/output", []string{"google"}, true)

	// Calculate hash twice - should be identical
	hash1, err := generator.calculateHash(pkg)
	if err != nil {
		t.Fatalf("Error calculating hash: %v", err)
	}

	hash2, err := generator.calculateHash(pkg)
	if err != nil {
		t.Fatalf("Error calculating hash: %v", err)
	}

	if hash1 != hash2 {
		t.Errorf("Hash should be deterministic, got %s and %s", hash1, hash2)
	}

	// Test that different inputs produce different hashes
	generator2 := NewProtoGenerator("test/output", []string{"google", "sf"}, true)
	hash3, err := generator2.calculateHash(pkg)
	if err != nil {
		t.Fatalf("Error calculating hash: %v", err)
	}

	if hash1 == hash3 {
		t.Errorf("Different exclude paths should produce different hashes")
	}

	// Test generateMod flag affects hash
	generator3 := NewProtoGenerator("test/output", []string{"google"}, false)
	hash4, err := generator3.calculateHash(pkg)
	if err != nil {
		t.Fatalf("Error calculating hash: %v", err)
	}

	if hash1 == hash4 {
		t.Errorf("Different generateMod values should produce different hashes")
	}
}

func TestHashFileOperations(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "proto_generator_test")
	if err != nil {
		t.Fatalf("Error creating temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	generator := NewProtoGenerator(tmpDir, []string{}, true)

	// Test reading non-existent hash file
	hash, err := generator.readLastGeneratedHash()
	if err != nil {
		t.Fatalf("Error reading non-existent hash file: %v", err)
	}
	if hash != "" {
		t.Errorf("Expected empty hash for non-existent file, got %s", hash)
	}

	// Test writing and reading hash file
	testHash := "abcdef123456"
	err = generator.writeLastGeneratedHash(testHash)
	if err != nil {
		t.Fatalf("Error writing hash file: %v", err)
	}

	readHash, err := generator.readLastGeneratedHash()
	if err != nil {
		t.Fatalf("Error reading hash file: %v", err)
	}

	if readHash != testHash {
		t.Errorf("Expected hash %s, got %s", testHash, readHash)
	}

	// Verify hash file exists
	hashFilePath := filepath.Join(tmpDir, ".last_generated_hash")
	if _, err := os.Stat(hashFilePath); os.IsNotExist(err) {
		t.Errorf("Hash file should exist at %s", hashFilePath)
	}
}

func TestHasGeneratedFiles(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "proto_generator_test")
	if err != nil {
		t.Fatalf("Error creating temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	generator := NewProtoGenerator(tmpDir, []string{}, true)

	// Test empty directory
	if generator.hasGeneratedFiles() {
		t.Errorf("Empty directory should not have generated files")
	}

	// Create a non-Rust file
	txtFile := filepath.Join(tmpDir, "test.txt")
	err = os.WriteFile(txtFile, []byte("test"), 0644)
	if err != nil {
		t.Fatalf("Error creating test file: %v", err)
	}

	if generator.hasGeneratedFiles() {
		t.Errorf("Directory with only non-Rust files should not have generated files")
	}

	// Create a Rust file
	rsFile := filepath.Join(tmpDir, "test.rs")
	err = os.WriteFile(rsFile, []byte("// Generated Rust code"), 0644)
	if err != nil {
		t.Fatalf("Error creating Rust file: %v", err)
	}

	if !generator.hasGeneratedFiles() {
		t.Errorf("Directory with Rust files should have generated files")
	}
}

func TestNonDeterministicDescriptors(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "proto_generator_test")
	if err != nil {
		t.Fatalf("Error creating temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a test proto file
	testProtoFile := &descriptorpb.FileDescriptorProto{
		Name:    stringPtr("test.proto"),
		Package: stringPtr("test"),
	}

	pkg := &pbsubstreams.Package{
		ProtoFiles:  []*descriptorpb.FileDescriptorProto{testProtoFile},
		PackageMeta: []*pbsubstreams.PackageMetadata{{Name: "test"}},
	}

	t.Run("deterministic descriptors write hash file", func(t *testing.T) {
		outputDir := filepath.Join(tmpDir, "deterministic")
		if err := os.MkdirAll(outputDir, 0755); err != nil {
			t.Fatalf("Error creating output dir: %v", err)
		}

		generator := NewProtoGenerator(outputDir, []string{}, true)
		generator.SetHasNonDeterministicDescriptors(false)

		// Write a hash file manually to test that it gets written (not deleted)
		testHash := "test_hash_value"
		if err := generator.writeLastGeneratedHash(testHash); err != nil {
			t.Fatalf("Error writing test hash: %v", err)
		}

		// Verify hash file exists
		hashFilePath := filepath.Join(outputDir, ".last_generated_hash")
		if _, err := os.Stat(hashFilePath); os.IsNotExist(err) {
			t.Errorf("Hash file should exist for deterministic descriptors")
		}
	})

	t.Run("non-deterministic descriptors delete hash file", func(t *testing.T) {
		outputDir := filepath.Join(tmpDir, "non_deterministic")
		if err := os.MkdirAll(outputDir, 0755); err != nil {
			t.Fatalf("Error creating output dir: %v", err)
		}

		generator := NewProtoGenerator(outputDir, []string{}, true)
		generator.SetHasNonDeterministicDescriptors(true)

		// Write a hash file manually first
		hashFilePath := filepath.Join(outputDir, ".last_generated_hash")
		if err := os.WriteFile(hashFilePath, []byte("old_hash"), 0644); err != nil {
			t.Fatalf("Error writing test hash: %v", err)
		}

		// Note: We can't easily test GenerateProto without buf installed,
		// but we can verify the flag is set correctly
		if !generator.hasNonDeterministicDescriptors {
			t.Errorf("hasNonDeterministicDescriptors should be true")
		}
	})

	t.Run("SetHasNonDeterministicDescriptors sets flag correctly", func(t *testing.T) {
		generator := NewProtoGenerator(tmpDir, []string{}, true)

		// Default should be false
		if generator.hasNonDeterministicDescriptors {
			t.Errorf("Default hasNonDeterministicDescriptors should be false")
		}

		generator.SetHasNonDeterministicDescriptors(true)
		if !generator.hasNonDeterministicDescriptors {
			t.Errorf("hasNonDeterministicDescriptors should be true after setting")
		}

		generator.SetHasNonDeterministicDescriptors(false)
		if generator.hasNonDeterministicDescriptors {
			t.Errorf("hasNonDeterministicDescriptors should be false after setting")
		}
	})

	t.Run("non-deterministic descriptors skip hash comparison", func(t *testing.T) {
		outputDir := filepath.Join(tmpDir, "skip_comparison")
		if err := os.MkdirAll(outputDir, 0755); err != nil {
			t.Fatalf("Error creating output dir: %v", err)
		}

		generator := NewProtoGenerator(outputDir, []string{}, true)

		// Write a matching hash file (to test that it would normally be skipped)
		currentHash, err := generator.calculateHash(pkg)
		if err != nil {
			t.Fatalf("Error calculating hash: %v", err)
		}
		if err := generator.writeLastGeneratedHash(currentHash); err != nil {
			t.Fatalf("Error writing hash: %v", err)
		}

		// Create a fake .rs file to simulate existing generated files
		rsFile := filepath.Join(outputDir, "test.rs")
		if err := os.WriteFile(rsFile, []byte("// test"), 0644); err != nil {
			t.Fatalf("Error creating rs file: %v", err)
		}

		// With deterministic descriptors, this should skip (hash matches, files exist)
		// But we can't fully test without buf, so we just verify the flag behavior
		generator.SetHasNonDeterministicDescriptors(false)
		lastHash, _ := generator.readLastGeneratedHash()
		if lastHash != currentHash {
			t.Errorf("Last hash should match current hash")
		}

		// Verify that hasGeneratedFiles returns true
		if !generator.hasGeneratedFiles() {
			t.Errorf("Should detect generated files")
		}
	})
}

// Helper function to create string pointers
func stringPtr(s string) *string {
	return &s
}

// pkgWith builds a Package descriptor from (protoPath, package) pairs, mirroring what
// the manifest hands GenerateProto.
func pkgWith(pairs ...[2]string) *pbsubstreams.Package {
	pkg := &pbsubstreams.Package{}
	for _, pair := range pairs {
		pkg.ProtoFiles = append(pkg.ProtoFiles, &descriptorpb.FileDescriptorProto{
			Name:    stringPtr(pair[0]),
			Package: stringPtr(pair[1]),
		})
	}
	return pkg
}

func touchModFiles(t *testing.T, outDir string, packages ...string) {
	t.Helper()
	if err := os.MkdirAll(outDir, 0755); err != nil {
		t.Fatalf("mkdir %q: %v", outDir, err)
	}
	for _, pkg := range packages {
		path := filepath.Join(outDir, pkg+".mod.rs")
		if err := os.WriteFile(path, []byte("// generated by buffa\n"), 0644); err != nil {
			t.Fatalf("write %q: %v", path, err)
		}
	}
}

// Every package buffa emits has to be wired up, transitive imports included: a
// cross-package type is referenced through a relative `super::` path, so omitting
// a package breaks the packages that reference it.
func TestWriteModuleTreeIncludesEveryEmittedPackage(t *testing.T) {
	out := filepath.Join(t.TempDir(), "src", "pb")
	touchModFiles(t, out, "mydata.v1", "sf.firehose.v2", "google.protobuf")
	pkg := pkgWith(
		[2]string{"mydata/v1/events.proto", "mydata.v1"},
		[2]string{"sf/firehose/v2/firehose.proto", "sf.firehose.v2"},
		[2]string{"google/protobuf/timestamp.proto", "google.protobuf"},
	)

	if err := NewProtoGenerator(out, nil, true).writeModuleTree(pkg); err != nil {
		t.Fatalf("writeModuleTree: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(out, "mod.rs"))
	if err != nil {
		t.Fatalf("reading mod.rs: %v", err)
	}
	for _, want := range []string{"mydata.v1.mod.rs", "sf.firehose.v2.mod.rs", "google.protobuf.mod.rs"} {
		if !strings.Contains(string(got), want) {
			t.Errorf("expected %q to be wired up, got:\n%s", want, got)
		}
	}
}

func TestWriteModuleTreeNestsSiblingPackages(t *testing.T) {
	out := filepath.Join(t.TempDir(), "src", "pb")
	touchModFiles(t, out, "a.b.c", "a.b.d", "solo")
	pkg := pkgWith(
		[2]string{"a/b/c.proto", "a.b.c"},
		[2]string{"a/b/d.proto", "a.b.d"},
		[2]string{"solo/solo.proto", "solo"},
	)

	if err := NewProtoGenerator(out, nil, true).writeModuleTree(pkg); err != nil {
		t.Fatalf("writeModuleTree: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(out, "mod.rs"))
	if err != nil {
		t.Fatalf("reading mod.rs: %v", err)
	}

	want := `// @generated by substreams
#![allow(clippy::all)]
pub mod a {
    pub mod b {
        pub mod c {
            include!("a.b.c.mod.rs");
        }
        pub mod d {
            include!("a.b.d.mod.rs");
        }
    }
}
pub mod solo {
    include!("solo.mod.rs");
}
`
	if string(got) != want {
		t.Errorf("mod.rs mismatch\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestWriteModuleTreeLeavesHandWrittenFileAlone(t *testing.T) {
	out := filepath.Join(t.TempDir(), "src", "pb")
	touchModFiles(t, out, "mydata.v1")

	handWritten := "#[allow(clippy::all)]\npub mod mydata {\n    pub mod v1 {\n        include!(\"mydata.v1.mod.rs\");\n    }\n}\n"
	modPath := filepath.Join(out, "mod.rs")
	if err := os.WriteFile(modPath, []byte(handWritten), 0644); err != nil {
		t.Fatalf("seeding mod.rs: %v", err)
	}

	pkg := pkgWith([2]string{"mydata/v1/events.proto", "mydata.v1"})
	if err := NewProtoGenerator(out, nil, true).writeModuleTree(pkg); err != nil {
		t.Fatalf("writeModuleTree: %v", err)
	}

	got, err := os.ReadFile(modPath)
	if err != nil {
		t.Fatalf("reading mod.rs: %v", err)
	}
	if string(got) != handWritten {
		t.Errorf("hand-written mod.rs was overwritten\n--- got ---\n%s", got)
	}
}

// A prost-crate generated tree starts with a bare `// @generated`, not our header,
// so it counts as hand-maintained and must survive too.
func TestWriteModuleTreeLeavesProstCrateFileAlone(t *testing.T) {
	out := filepath.Join(t.TempDir(), "src", "pb")
	touchModFiles(t, out, "mydata.v1", "sf.firehose.v2")

	prostCrate := "// @generated\npub mod mydata {\n    pub mod v1 {\n        include!(\"mydata.v1.rs\");\n    }\n}\n"
	modPath := filepath.Join(out, "mod.rs")
	if err := os.WriteFile(modPath, []byte(prostCrate), 0644); err != nil {
		t.Fatalf("seeding mod.rs: %v", err)
	}

	pkg := pkgWith([2]string{"mydata/v1/events.proto", "mydata.v1"})
	if err := NewProtoGenerator(out, nil, true).writeModuleTree(pkg); err != nil {
		t.Fatalf("writeModuleTree: %v", err)
	}

	got, err := os.ReadFile(modPath)
	if err != nil {
		t.Fatalf("reading mod.rs: %v", err)
	}
	if string(got) != prostCrate {
		t.Errorf("a prost-crate mod.rs was overwritten\n--- got ---\n%s", got)
	}
}

// Our own output carries the header, so a new package must make it into the tree
// rather than leaving a stale file in place.
func TestWriteModuleTreeRefreshesItsOwnOutput(t *testing.T) {
	out := filepath.Join(t.TempDir(), "src", "pb")
	touchModFiles(t, out, "mydata.v1")

	generator := NewProtoGenerator(out, nil, true)
	one := pkgWith([2]string{"mydata/v1/events.proto", "mydata.v1"})
	if err := generator.writeModuleTree(one); err != nil {
		t.Fatalf("first writeModuleTree: %v", err)
	}

	touchModFiles(t, out, "extra.v1")
	two := pkgWith(
		[2]string{"mydata/v1/events.proto", "mydata.v1"},
		[2]string{"extra/v1/extra.proto", "extra.v1"},
	)
	if err := generator.writeModuleTree(two); err != nil {
		t.Fatalf("second writeModuleTree: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(out, "mod.rs"))
	if err != nil {
		t.Fatalf("reading mod.rs: %v", err)
	}
	if !strings.Contains(string(got), "extra.v1.mod.rs") {
		t.Errorf("a newly emitted package was not picked up, got:\n%s", got)
	}
}

// Regeneration in place, without removing the file first, must be byte-identical.
func TestWriteModuleTreeIsIdempotent(t *testing.T) {
	out := filepath.Join(t.TempDir(), "src", "pb")
	touchModFiles(t, out, "mydata.v1", "sf.firehose.v2")

	generator := NewProtoGenerator(out, nil, true)
	pkg := pkgWith(
		[2]string{"mydata/v1/events.proto", "mydata.v1"},
		[2]string{"sf/firehose/v2/firehose.proto", "sf.firehose.v2"},
	)
	if err := generator.writeModuleTree(pkg); err != nil {
		t.Fatalf("first writeModuleTree: %v", err)
	}
	first, err := os.ReadFile(filepath.Join(out, "mod.rs"))
	if err != nil {
		t.Fatalf("reading mod.rs: %v", err)
	}

	if err := generator.writeModuleTree(pkg); err != nil {
		t.Fatalf("second writeModuleTree: %v", err)
	}
	second, err := os.ReadFile(filepath.Join(out, "mod.rs"))
	if err != nil {
		t.Fatalf("reading mod.rs: %v", err)
	}

	if string(first) != string(second) {
		t.Errorf("not idempotent\n--- first ---\n%s\n--- second ---\n%s", first, second)
	}
}

func TestWriteModuleTreeNoGeneratedPackages(t *testing.T) {
	out := filepath.Join(t.TempDir(), "src", "pb")
	if err := os.MkdirAll(out, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	if err := NewProtoGenerator(out, nil, true).writeModuleTree(pkgWith()); err != nil {
		t.Fatalf("writeModuleTree: %v", err)
	}
	if _, err := os.Stat(filepath.Join(out, "mod.rs")); !os.IsNotExist(err) {
		t.Errorf("expected no mod.rs when buffa emitted nothing, stat err = %v", err)
	}
}

// `buf` never removes stale output, so a package whose `.proto` was deleted or renamed
// still has a `.mod.rs` on disk. Wiring it back up breaks the build.
func TestWriteModuleTreeIgnoresStaleModFiles(t *testing.T) {
	out := filepath.Join(t.TempDir(), "src", "pb")
	touchModFiles(t, out, "a", "a.b")

	// a.b was removed from the manifest; only `a` remains.
	pkg := pkgWith([2]string{"a/a.proto", "a"})

	if err := NewProtoGenerator(out, nil, true).writeModuleTree(pkg); err != nil {
		t.Fatalf("writeModuleTree: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(out, "mod.rs"))
	if err != nil {
		t.Fatalf("reading mod.rs: %v", err)
	}
	if strings.Contains(string(got), "a.b.mod.rs") {
		t.Errorf("a stale package was wired up, got:\n%s", got)
	}
	if !strings.Contains(string(got), `include!("a.mod.rs")`) {
		t.Errorf("the live package is missing, got:\n%s", got)
	}
}

// buffa writes a descriptor with no `package` to `__buffa.mod.rs` and expects it at the
// root; nesting it under `pub mod __buffa` does not compile.
func TestWriteModuleTreePlacesPackagelessFilesAtRoot(t *testing.T) {
	out := filepath.Join(t.TempDir(), "src", "pb")
	touchModFiles(t, out, "__buffa", "mydata.v1")

	pkg := pkgWith(
		[2]string{"nopkg.proto", ""},
		[2]string{"mydata/v1/events.proto", "mydata.v1"},
	)

	if err := NewProtoGenerator(out, nil, true).writeModuleTree(pkg); err != nil {
		t.Fatalf("writeModuleTree: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(out, "mod.rs"))
	if err != nil {
		t.Fatalf("reading mod.rs: %v", err)
	}
	if strings.Contains(string(got), "pub mod __buffa {") {
		t.Errorf("the package-less file was nested instead of placed at the root, got:\n%s", got)
	}
	if !strings.Contains(string(got), "\ninclude!(\"__buffa.mod.rs\");\n") {
		t.Errorf("expected a root-level include for __buffa, got:\n%s", got)
	}
}

// Rewriting an unchanged mod.rs bumps its timestamp and forces a needless recompile.
func TestWriteModuleTreeDoesNotRewriteUnchangedContent(t *testing.T) {
	out := filepath.Join(t.TempDir(), "src", "pb")
	touchModFiles(t, out, "mydata.v1")
	pkg := pkgWith([2]string{"mydata/v1/events.proto", "mydata.v1"})

	generator := NewProtoGenerator(out, nil, true)
	if err := generator.writeModuleTree(pkg); err != nil {
		t.Fatalf("first writeModuleTree: %v", err)
	}

	modPath := filepath.Join(out, "mod.rs")
	before, err := os.Stat(modPath)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}

	// Backdate so any rewrite is visible regardless of clock granularity.
	old := before.ModTime().Add(-time.Hour)
	if err := os.Chtimes(modPath, old, old); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	if err := generator.writeModuleTree(pkg); err != nil {
		t.Fatalf("second writeModuleTree: %v", err)
	}

	after, err := os.Stat(modPath)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if !after.ModTime().Equal(old) {
		t.Errorf("unchanged mod.rs was rewritten: mtime moved from %v to %v", old, after.ModTime())
	}
}

// A package listed in the descriptors but never emitted by buffa must not be wired up.
func TestWriteModuleTreeSkipsPackagesWithoutOutput(t *testing.T) {
	out := filepath.Join(t.TempDir(), "src", "pb")
	touchModFiles(t, out, "mydata.v1")

	pkg := pkgWith(
		[2]string{"mydata/v1/events.proto", "mydata.v1"},
		[2]string{"ghost/v1/ghost.proto", "ghost.v1"},
	)

	if err := NewProtoGenerator(out, nil, true).writeModuleTree(pkg); err != nil {
		t.Fatalf("writeModuleTree: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(out, "mod.rs"))
	if err != nil {
		t.Fatalf("reading mod.rs: %v", err)
	}
	if strings.Contains(string(got), "ghost") {
		t.Errorf("wired up a package with no generated file, got:\n%s", got)
	}
}

// An excluded path is never generated, so buffa emits no module file for it and the
// tree leaves it out without needing to reimplement buf's path matching.
func TestWriteModuleTreeOmitsPackagesBufDidNotGenerate(t *testing.T) {
	out := filepath.Join(t.TempDir(), "src", "pb")
	// Only the module's own package was generated; sf.substreams was excluded.
	touchModFiles(t, out, "mydata.v1")

	pkg := pkgWith(
		[2]string{"mydata/v1/events.proto", "mydata.v1"},
		[2]string{"sf/substreams/v1/clock.proto", "sf.substreams.v1"},
	)

	generator := NewProtoGenerator(out, []string{"sf/substreams"}, true)
	if err := generator.writeModuleTree(pkg); err != nil {
		t.Fatalf("writeModuleTree: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(out, "mod.rs"))
	if err != nil {
		t.Fatalf("reading mod.rs: %v", err)
	}
	if strings.Contains(string(got), "sf.substreams.v1.mod.rs") {
		t.Errorf("wired up a package buffa never generated, got:\n%s", got)
	}
	if !strings.Contains(string(got), `include!("mydata.v1.mod.rs")`) {
		t.Errorf("the generated package is missing, got:\n%s", got)
	}
}

// A package split across several `.proto` files is an ordinary layout. buffa merges
// them into one `<pkg>.mod.rs`, so the tree must include it exactly once; a second
// `include!` defines every type in the package twice and the crate stops compiling.
func TestWriteModuleTreeIncludesEachPackageOnce(t *testing.T) {
	out := filepath.Join(t.TempDir(), "src", "pb")
	if err := os.MkdirAll(out, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	touchModFiles(t, out, "a.v1")

	pkg := pkgWith([2]string{"a/v1/one.proto", "a.v1"}, [2]string{"a/v1/two.proto", "a.v1"})
	if err := NewProtoGenerator(out, nil, true).writeModuleTree(pkg); err != nil {
		t.Fatalf("writeModuleTree: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(out, "mod.rs"))
	if err != nil {
		t.Fatalf("reading mod.rs: %v", err)
	}
	if got := strings.Count(string(content), `include!("a.v1.mod.rs")`); got != 1 {
		t.Errorf("include count = %d, want 1\n%s", got, content)
	}
}

// moduleTreeIsComplete is what makes the generation-hash cache trustworthy: the hash
// covers the inputs, so it still matches after the generated output has been removed
// by a clean, a .gitignore, or an interrupted run. Skipping generation then leaves a
// crate that cannot compile.
func TestModuleTreeIsComplete(t *testing.T) {
	const ourMod = generatedModHeader + "\npub mod a { pub mod v1 { include!(\"a.v1.mod.rs\"); } }\n"

	for _, tt := range []struct {
		name     string
		modFile  string   // contents of mod.rs, "" to leave it absent
		emitted  []string // packages buf has generated a file for
		descript *pbsubstreams.Package
		want     bool
	}{
		{
			name:     "ours and fully generated",
			modFile:  ourMod,
			emitted:  []string{"a.v1"},
			descript: pkgWith([2]string{"a/v1/a.proto", "a.v1"}),
			want:     true,
		},
		{
			// The case the function exists for.
			name:     "ours but the generated output is gone",
			modFile:  ourMod,
			emitted:  nil,
			descript: pkgWith([2]string{"a/v1/a.proto", "a.v1"}),
			want:     false,
		},
		{
			// writeModuleTree restores it from the files already on disk, so this is
			// not a reason to rerun buf.
			name:     "mod.rs missing but the packages are generated",
			modFile:  "",
			emitted:  []string{"a.v1"},
			descript: pkgWith([2]string{"a/v1/a.proto", "a.v1"}),
			want:     true,
		},
		{
			// Someone else's file is their business; never force a regeneration on it.
			name:     "hand written mod.rs",
			modFile:  "// mine\n",
			emitted:  nil,
			descript: pkgWith([2]string{"a/v1/a.proto", "a.v1"}),
			want:     true,
		},
		{
			name:     "descriptor declares no proto files",
			modFile:  ourMod,
			emitted:  nil,
			descript: pkgWith(),
			want:     true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			out := filepath.Join(t.TempDir(), "src", "pb")
			if err := os.MkdirAll(out, 0755); err != nil {
				t.Fatalf("mkdir: %v", err)
			}
			if tt.modFile != "" {
				if err := os.WriteFile(filepath.Join(out, "mod.rs"), []byte(tt.modFile), 0644); err != nil {
					t.Fatalf("seeding mod.rs: %v", err)
				}
			}
			touchModFiles(t, out, tt.emitted...)

			got, err := NewProtoGenerator(out, nil, true).moduleTreeIsComplete(tt.descript)
			if err != nil {
				t.Fatalf("moduleTreeIsComplete: %v", err)
			}
			if got != tt.want {
				t.Errorf("moduleTreeIsComplete = %v, want %v", got, tt.want)
			}
		})
	}
}

// An unreadable mod.rs must surface rather than be guessed at in either direction.
func TestModuleTreeIsCompleteSurfacesReadErrors(t *testing.T) {
	out := filepath.Join(t.TempDir(), "src", "pb")
	if err := os.MkdirAll(filepath.Join(out, "mod.rs"), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	if _, err := NewProtoGenerator(out, nil, true).moduleTreeIsComplete(pkgWith()); err == nil {
		t.Error("expected an error when mod.rs cannot be read")
	}
}

// writeModuleTree must not swallow a failed write: reporting success over a crate with
// no mod.rs turns a clear failure into a confusing cargo error later.
func TestWriteModuleTreeReportsWriteFailures(t *testing.T) {
	out := filepath.Join(t.TempDir(), "src", "pb")
	if err := os.MkdirAll(filepath.Join(out, "mod.rs"), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	touchModFiles(t, out, "a.v1")

	err := NewProtoGenerator(out, nil, true).writeModuleTree(pkgWith([2]string{"a/v1/a.proto", "a.v1"}))
	if err == nil {
		t.Error("expected an error when mod.rs cannot be written")
	}
}

// The generation cache keys on the inputs, so it still matches after the generated
// output has gone. Skipping then leaves a crate that cannot compile, and re-running
// never repairs it because the cache keeps hitting.
func TestCanSkipGeneration(t *testing.T) {
	descriptor := pkgWith([2]string{"a/v1/a.proto", "a.v1"})

	for _, tt := range []struct {
		name        string
		generateMod bool
		writeHash   bool
		modFile     string
		emitted     []string
		want        bool
	}{
		{
			name:        "everything present",
			generateMod: true,
			writeHash:   true,
			modFile:     generatedModHeader + "\npub mod a { pub mod v1 { include!(\"a.v1.mod.rs\"); } }\n",
			emitted:     []string{"a.v1"},
			want:        true,
		},
		{
			// The regression this guards: hash matches, a stray .rs satisfies
			// hasGeneratedFiles, but the package output is gone.
			name:        "hash matches but the package output was removed",
			generateMod: true,
			writeHash:   true,
			modFile:     generatedModHeader + "\npub mod a { pub mod v1 { include!(\"a.v1.mod.rs\"); } }\n",
			emitted:     nil,
			want:        false,
		},
		{
			name:        "no hash recorded yet",
			generateMod: true,
			writeHash:   false,
			modFile:     "",
			emitted:     []string{"a.v1"},
			want:        false,
		},
		{
			// Without mod.rs generation there is nothing extra to verify.
			name:        "mod.rs generation disabled",
			generateMod: false,
			writeHash:   true,
			modFile:     "",
			emitted:     []string{"a.v1"},
			want:        true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			out := filepath.Join(t.TempDir(), "src", "pb")
			if err := os.MkdirAll(out, 0755); err != nil {
				t.Fatalf("mkdir: %v", err)
			}
			// hasGeneratedFiles only looks for any *.rs, so keep one around to make
			// sure the completeness check is what decides, not that guard.
			if err := os.WriteFile(filepath.Join(out, "a.rs"), []byte("// types\n"), 0644); err != nil {
				t.Fatalf("seeding a.rs: %v", err)
			}
			if tt.modFile != "" {
				if err := os.WriteFile(filepath.Join(out, "mod.rs"), []byte(tt.modFile), 0644); err != nil {
					t.Fatalf("seeding mod.rs: %v", err)
				}
			}
			touchModFiles(t, out, tt.emitted...)

			generator := NewProtoGenerator(out, nil, tt.generateMod)
			hash, err := generator.calculateHash(descriptor)
			if err != nil {
				t.Fatalf("calculateHash: %v", err)
			}
			if tt.writeHash {
				if err := generator.writeLastGeneratedHash(hash); err != nil {
					t.Fatalf("writeLastGeneratedHash: %v", err)
				}
			}

			got, err := generator.canSkipGeneration(descriptor, hash)
			if err != nil {
				t.Fatalf("canSkipGeneration: %v", err)
			}
			if got != tt.want {
				t.Errorf("canSkipGeneration = %v, want %v", got, tt.want)
			}
		})
	}
}

// A non-deterministic descriptor set can change without changing the hash, so the
// cache must never be trusted for one.
func TestCanSkipGenerationNeverSkipsNonDeterministicDescriptors(t *testing.T) {
	out := filepath.Join(t.TempDir(), "src", "pb")
	if err := os.MkdirAll(out, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	touchModFiles(t, out, "a.v1")

	descriptor := pkgWith([2]string{"a/v1/a.proto", "a.v1"})
	generator := NewProtoGenerator(out, nil, true)
	hash, err := generator.calculateHash(descriptor)
	if err != nil {
		t.Fatalf("calculateHash: %v", err)
	}
	if err := generator.writeLastGeneratedHash(hash); err != nil {
		t.Fatalf("writeLastGeneratedHash: %v", err)
	}

	generator.SetHasNonDeterministicDescriptors(true)

	skip, err := generator.canSkipGeneration(descriptor, hash)
	if err != nil {
		t.Fatalf("canSkipGeneration: %v", err)
	}
	if skip {
		t.Error("generation was skipped for a non-deterministic descriptor set")
	}
}
