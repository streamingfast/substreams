package manifest

import (
	"os"
	"path/filepath"
	"testing"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
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
