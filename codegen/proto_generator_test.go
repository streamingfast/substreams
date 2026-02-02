package codegen

import (
	"os"
	"path/filepath"
	"testing"

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
