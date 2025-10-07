package manifest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
)

func TestLoadProtobufFromDirectory(t *testing.T) {
	// Create a temporary directory with a proto file
	tempDir, err := os.MkdirTemp("", "proto_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	// Create a simple proto file
	protoContent := `syntax = "proto3";
package test;

message TestMessage {
  string id = 1;
  string name = 2;
}
`
	protoFile := filepath.Join(tempDir, "test.proto")
	err = os.WriteFile(protoFile, []byte(protoContent), 0644)
	if err != nil {
		t.Fatal(err)
	}

	// Create a package to load into
	pkg := &pbsubstreams.Package{
		ProtoFiles: []*descriptorpb.FileDescriptorProto{},
	}

	// Load protobuf from directory
	descriptors, err := loadProtobufFromDirectory(pkg, tempDir)
	if err != nil {
		t.Fatalf("Failed to load protobuf from directory: %v", err)
	}

	// Verify that we got descriptors
	if len(descriptors) == 0 {
		t.Fatal("Expected at least one descriptor, got none")
	}

	// Verify that the proto file was added to the package
	if len(pkg.ProtoFiles) == 0 {
		t.Fatal("Expected proto files to be added to package")
	}

	// Verify the proto file name
	found := false
	for _, protoFile := range pkg.ProtoFiles {
		if protoFile.GetName() == "test.proto" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("Expected to find test.proto in package proto files")
	}
}

func TestLoadProtobufFromDirectory_EmptyDirectory(t *testing.T) {
	// Create a temporary empty directory
	tempDir, err := os.MkdirTemp("", "proto_test_empty")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	// Create a package to load into
	pkg := &pbsubstreams.Package{
		ProtoFiles: []*descriptorpb.FileDescriptorProto{},
	}

	// Load protobuf from empty directory
	descriptors, err := loadProtobufFromDirectory(pkg, tempDir)
	if err != nil {
		t.Fatalf("Failed to load protobuf from empty directory: %v", err)
	}

	// Should return nil descriptors for empty directory
	if descriptors != nil {
		t.Fatal("Expected nil descriptors for empty directory")
	}
}

func TestLoadProtobufFromDescriptorSet(t *testing.T) {
	// Create a temporary file with a descriptor set
	tempFile, err := os.CreateTemp("", "descriptor_set_test*.desc")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tempFile.Name())

	// Create a simple FileDescriptorSet
	fds := &descriptorpb.FileDescriptorSet{
		File: []*descriptorpb.FileDescriptorProto{
			{
				Name:    proto.String("test.proto"),
				Package: proto.String("test"),
				MessageType: []*descriptorpb.DescriptorProto{
					{
						Name: proto.String("TestMessage"),
						Field: []*descriptorpb.FieldDescriptorProto{
							{
								Name:   proto.String("id"),
								Number: proto.Int32(1),
								Type:   descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
							},
						},
					},
				},
			},
		},
	}

	// Marshal and write to file
	data, err := proto.Marshal(fds)
	if err != nil {
		t.Fatal(err)
	}

	_, err = tempFile.Write(data)
	if err != nil {
		t.Fatal(err)
	}
	tempFile.Close()

	// Create a package to load into
	pkg := &pbsubstreams.Package{
		ProtoFiles: []*descriptorpb.FileDescriptorProto{},
	}

	// Load protobuf from descriptor set
	descriptors, err := loadProtobufFromDescriptorSet(pkg, tempFile.Name())
	if err != nil {
		t.Fatalf("Failed to load protobuf from descriptor set: %v", err)
	}

	// Verify that we got descriptors
	if len(descriptors) == 0 {
		t.Fatal("Expected at least one descriptor, got none")
	}

	// Verify that the proto file was added to the package
	if len(pkg.ProtoFiles) == 0 {
		t.Fatal("Expected proto files to be added to package")
	}

	// Verify the proto file name
	found := false
	for _, protoFile := range pkg.ProtoFiles {
		if protoFile.GetName() == "test.proto" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("Expected to find test.proto in package proto files")
	}
}

func TestLoadProtobufFromDescriptorSet_InvalidFile(t *testing.T) {
	// Create a temporary file with invalid content
	tempFile, err := os.CreateTemp("", "invalid_descriptor_set_test*.desc")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tempFile.Name())

	// Write invalid content
	_, err = tempFile.WriteString("invalid content")
	if err != nil {
		t.Fatal(err)
	}
	tempFile.Close()

	// Create a package to load into
	pkg := &pbsubstreams.Package{
		ProtoFiles: []*descriptorpb.FileDescriptorProto{},
	}

	// Load protobuf from invalid descriptor set should fail
	_, err = loadProtobufFromDescriptorSet(pkg, tempFile.Name())
	if err == nil {
		t.Fatal("Expected error when loading invalid descriptor set")
	}
}

func TestLoadProtobufFromDescriptorSet_NonexistentFile(t *testing.T) {
	// Create a package to load into
	pkg := &pbsubstreams.Package{
		ProtoFiles: []*descriptorpb.FileDescriptorProto{},
	}

	// Load protobuf from nonexistent file should fail
	_, err := loadProtobufFromDescriptorSet(pkg, "/nonexistent/file.desc")
	if err == nil {
		t.Fatal("Expected error when loading from nonexistent file")
	}
}

func TestReader_LoadAdditionalProtobufs(t *testing.T) {
	// Create a temporary directory with a proto file
	tempDir, err := os.MkdirTemp("", "reader_proto_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	// Create a simple proto file
	protoContent := `syntax = "proto3";
package reader_test;

message ReaderTestMessage {
  string id = 1;
  int64 value = 2;
}
`
	protoFile := filepath.Join(tempDir, "reader_test.proto")
	err = os.WriteFile(protoFile, []byte(protoContent), 0644)
	if err != nil {
		t.Fatal(err)
	}

	// Create a descriptor set file
	tempDescFile, err := os.CreateTemp("", "reader_desc_test*.desc")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tempDescFile.Name())

	fds := &descriptorpb.FileDescriptorSet{
		File: []*descriptorpb.FileDescriptorProto{
			{
				Name:    proto.String("reader_desc_test.proto"),
				Package: proto.String("reader_desc_test"),
				MessageType: []*descriptorpb.DescriptorProto{
					{
						Name: proto.String("ReaderDescTestMessage"),
						Field: []*descriptorpb.FieldDescriptorProto{
							{
								Name:   proto.String("data"),
								Number: proto.Int32(1),
								Type:   descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
							},
						},
					},
				},
			},
		},
	}

	data, err := proto.Marshal(fds)
	if err != nil {
		t.Fatal(err)
	}

	_, err = tempDescFile.Write(data)
	if err != nil {
		t.Fatal(err)
	}
	tempDescFile.Close()

	// Create a pre-compiled package (simulate .spkg loading)
	pkg := &pbsubstreams.Package{
		ProtoFiles: []*descriptorpb.FileDescriptorProto{
			{
				Name:    proto.String("existing.proto"),
				Package: proto.String("existing"),
				MessageType: []*descriptorpb.DescriptorProto{
					{
						Name: proto.String("ExistingMessage"),
						Field: []*descriptorpb.FieldDescriptorProto{
							{
								Name:   proto.String("existing_field"),
								Number: proto.Int32(1),
								Type:   descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
							},
						},
					},
				},
			},
		},
	}

	// Create a reader with both proto path and descriptor set
	reader := &Reader{
		protoPath:          tempDir,
		protoDescriptorSet: tempDescFile.Name(),
	}

	// Load additional protobufs
	err = reader.loadAdditionalProtobufs(pkg)
	if err != nil {
		t.Fatalf("Failed to load additional protobufs: %v", err)
	}

	// Verify that all proto files are present
	expectedFiles := map[string]bool{
		"existing.proto":         false,
		"reader_test.proto":      false,
		"reader_desc_test.proto": false,
	}

	for _, protoFile := range pkg.ProtoFiles {
		if _, exists := expectedFiles[protoFile.GetName()]; exists {
			expectedFiles[protoFile.GetName()] = true
		}
	}

	for fileName, found := range expectedFiles {
		if !found {
			t.Errorf("Expected to find proto file %s, but it was not loaded", fileName)
		}
	}

	// Verify we have at least 3 proto files (original + 2 additional)
	if len(pkg.ProtoFiles) < 3 {
		t.Errorf("Expected at least 3 proto files, got %d", len(pkg.ProtoFiles))
	}
}

func TestReaderOptions_ProtoPathAndDescriptorSet(t *testing.T) {
	// Test that WithProtoPath and WithProtoDescriptorSet options work correctly
	reader := &Reader{}

	// Test WithProtoPath option
	protoPathOption := WithProtoPath("/test/proto/path")
	reader = protoPathOption(reader)

	if reader.protoPath != "/test/proto/path" {
		t.Errorf("Expected protoPath to be '/test/proto/path', got '%s'", reader.protoPath)
	}

	// Test WithProtoDescriptorSet option
	descriptorSetOption := WithProtoDescriptorSet("/test/descriptor/set.desc")
	reader = descriptorSetOption(reader)

	if reader.protoDescriptorSet != "/test/descriptor/set.desc" {
		t.Errorf("Expected protoDescriptorSet to be '/test/descriptor/set.desc', got '%s'", reader.protoDescriptorSet)
	}
}

// Cache-related tests
func TestDescriptorCache_GenerateCacheKey(t *testing.T) {
	cache := &DescriptorCache{cacheDir: t.TempDir()}

	tests := []struct {
		name    string
		module  string
		version string
		symbols []string
	}{
		{
			name:    "basic key generation",
			module:  "buf.build/streamingfast/substreams",
			version: "v1.0.0",
			symbols: []string{"proto.Message"},
		},
		{
			name:    "empty symbols",
			module:  "buf.build/test/module",
			version: "v2.1.0",
			symbols: []string{},
		},
		{
			name:    "multiple symbols",
			module:  "buf.build/multi/module",
			version: "v1.0.0",
			symbols: []string{"proto.A", "proto.B", "proto.C"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := cache.generateCacheKey(tt.module, tt.version, tt.symbols)

			// Verify it's a valid hex string of correct length (SHA256 = 64 chars)
			assert.Equal(t, 64, len(result), "Cache key should be 64 characters (SHA256 hex)")

			// Verify it's all hex characters
			for _, c := range result {
				assert.True(t, strings.ContainsRune("0123456789abcdef", c), "Cache key should only contain hex characters")
			}

			// Verify deterministic
			result2 := cache.generateCacheKey(tt.module, tt.version, tt.symbols)
			assert.Equal(t, result, result2, "Cache key generation should be deterministic")
		})
	}
}

func TestDescriptorCache_GenerateCacheKey_Uniqueness(t *testing.T) {
	cache := &DescriptorCache{cacheDir: t.TempDir()}

	// Test that different inputs produce different keys
	key1 := cache.generateCacheKey("module1", "v1.0.0", []string{"symbol1"})
	key2 := cache.generateCacheKey("module2", "v1.0.0", []string{"symbol1"})
	key3 := cache.generateCacheKey("module1", "v2.0.0", []string{"symbol1"})
	key4 := cache.generateCacheKey("module1", "v1.0.0", []string{"symbol2"})

	keys := map[string]bool{
		key1: true,
		key2: true,
		key3: true,
		key4: true,
	}

	assert.Equal(t, 4, len(keys), "All cache keys should be unique")
}

func TestDescriptorCache_IsDeterministicVersion(t *testing.T) {
	cache := &DescriptorCache{cacheDir: t.TempDir()}

	tests := []struct {
		version              string
		isDeterministicVersion bool
		description          string
	}{
		{"v1.0.0", true, "semantic version should be cached"},
		{"v2.1.3", true, "semantic version should be cached"},
		{"", false, "empty version should not be cached"},
		{"main", false, "main branch should not be cached"},
		{"develop", false, "develop branch should not be cached"},
		{"master", false, "master branch should not be cached"},
	}

	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			result := cache.isDeterministicVersion(tt.version)
			assert.Equal(t, tt.isDeterministicVersion, result, tt.description)
		})
	}
}

func TestDescriptorCache_SaveAndLoad(t *testing.T) {
	cache := &DescriptorCache{cacheDir: t.TempDir()}

	// Create test data
	testFds := &descriptorpb.FileDescriptorSet{
		File: []*descriptorpb.FileDescriptorProto{
			{
				Name:    proto.String("test_cache.proto"),
				Package: proto.String("test_cache"),
				MessageType: []*descriptorpb.DescriptorProto{
					{
						Name: proto.String("TestCacheMessage"),
						Field: []*descriptorpb.FieldDescriptorProto{
							{
								Name:   proto.String("cache_field"),
								Number: proto.Int32(1),
								Type:   descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
							},
						},
					},
				},
			},
		},
	}

	cacheKey := "test_cache_key_1234567890abcdef1234567890abcdef12345678"

	// Test save to cache
	err := cache.save(cacheKey, testFds)
	require.NoError(t, err, "Failed to save to cache")

	// Test load from cache
	loadedFds, err := cache.load(cacheKey)
	require.NoError(t, err, "Failed to load from cache")

	// Verify the loaded data matches the saved data using proto.Equal
	if !proto.Equal(testFds, loadedFds) {
		require.Equal(t, testFds, loadedFds, "Loaded descriptor should match saved descriptor")
	}

	// Additional checks
	assert.Equal(t, len(testFds.File), len(loadedFds.File), "File count should match")
	assert.Equal(t, testFds.File[0].GetName(), loadedFds.File[0].GetName(), "File name should match")
	assert.Equal(t, testFds.File[0].GetPackage(), loadedFds.File[0].GetPackage(), "Package name should match")
}

func TestDescriptorCache_Load_NotFound(t *testing.T) {
	cache := &DescriptorCache{cacheDir: t.TempDir()}

	// Try to load from cache with non-existent key
	_, err := cache.load("nonexistent_cache_key_1234567890abcdef1234567890abcdef")
	assert.Error(t, err, "Expected error when loading non-existent cache key")
}

func TestNewDescriptorCache(t *testing.T) {
	cache := newDescriptorCache()
	require.NotNil(t, cache, "Cache should not be nil")

	// Verify the directory exists
	_, err := os.Stat(cache.cacheDir)
	require.NoError(t, err, "Cache directory should exist")

	// Verify the path contains expected components
	assert.Contains(t, cache.cacheDir, "substreams", "Cache dir should contain 'substreams'")
	assert.Contains(t, cache.cacheDir, "buf-cache", "Cache dir should contain 'buf-cache'")
}

func TestCacheIntegration(t *testing.T) {
	cache := &DescriptorCache{cacheDir: t.TempDir()}

	// Create realistic test data similar to what buf.build would return
	testFds := &descriptorpb.FileDescriptorSet{
		File: []*descriptorpb.FileDescriptorProto{
			{
				Name:    proto.String("google/protobuf/descriptor.proto"),
				Package: proto.String("google.protobuf"),
				MessageType: []*descriptorpb.DescriptorProto{
					{
						Name: proto.String("FileDescriptorSet"),
						Field: []*descriptorpb.FieldDescriptorProto{
							{
								Name:     proto.String("file"),
								Number:   proto.Int32(1),
								Type:     descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
								TypeName: proto.String(".google.protobuf.FileDescriptorProto"),
								Label:    descriptorpb.FieldDescriptorProto_LABEL_REPEATED.Enum(),
							},
						},
					},
				},
			},
			{
				Name:    proto.String("sf/substreams/sink/service/v1/service.proto"),
				Package: proto.String("sf.substreams.sink.service.v1"),
				MessageType: []*descriptorpb.DescriptorProto{
					{
						Name: proto.String("DeployRequest"),
						Field: []*descriptorpb.FieldDescriptorProto{
							{
								Name:     proto.String("substreams_package"),
								Number:   proto.Int32(1),
								Type:     descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
								TypeName: proto.String(".sf.substreams.v1.Package"),
							},
						},
					},
				},
			},
		},
	}

	// Generate cache key for realistic module/version/symbols
	module := "buf.build/streamingfast/substreams-sink-sql"
	version := "v1.0.0"
	symbols := []string{
		"sf.substreams.sink.service.v1.DeployRequest",
		"sf.substreams.sink.service.v1.DeployResponse",
	}
	cacheKey := cache.generateCacheKey(module, version, symbols)

	// Test: First call should save to cache
	err := cache.save(cacheKey, testFds)
	require.NoError(t, err, "Failed to save realistic data to cache")

	// Test: Second call should load from cache
	cachedFds, err := cache.load(cacheKey)
	require.NoError(t, err, "Failed to load realistic data from cache")

	// Verify integrity of cached data using proto.Equal
	if !proto.Equal(testFds, cachedFds) {
		require.Equal(t, testFds, cachedFds, "Cached descriptor should match original")
	}
}
