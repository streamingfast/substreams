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

// Helper function to create string pointers
func stringPtr(s string) *string {
	return &s
}
