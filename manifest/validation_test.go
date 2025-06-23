package manifest

import (
	"testing"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"google.golang.org/protobuf/types/descriptorpb"
)

func TestValidateProtoOutputTypes(t *testing.T) {
	tests := []struct {
		name        string
		pkg         *pbsubstreams.Package
		expectError bool
		errorMsg    string
	}{
		{
			name: "no modules",
			pkg: &pbsubstreams.Package{
				Modules: nil,
			},
			expectError: false,
		},
		{
			name: "module without output",
			pkg: &pbsubstreams.Package{
				Modules: &pbsubstreams.Modules{
					Modules: []*pbsubstreams.Module{
						{
							Name:   "test_module",
							Output: nil,
						},
					},
				},
			},
			expectError: false,
		},
		{
			name: "module with non-proto output",
			pkg: &pbsubstreams.Package{
				Modules: &pbsubstreams.Modules{
					Modules: []*pbsubstreams.Module{
						{
							Name: "test_module",
							Output: &pbsubstreams.Module_Output{
								Type: "string",
							},
						},
					},
				},
			},
			expectError: false,
		},
		{
			name: "module with valid proto output",
			pkg: &pbsubstreams.Package{
				ProtoFiles: []*descriptorpb.FileDescriptorProto{
					{
						Name:    stringPtr("test.proto"),
						Package: stringPtr("test.package"),
						MessageType: []*descriptorpb.DescriptorProto{
							{
								Name: stringPtr("TestMessage"),
							},
						},
					},
				},
				Modules: &pbsubstreams.Modules{
					Modules: []*pbsubstreams.Module{
						{
							Name: "test_module",
							Output: &pbsubstreams.Module_Output{
								Type: "proto:test.package.TestMessage",
							},
						},
					},
				},
			},
			expectError: false,
		},
		{
			name: "module with invalid proto output",
			pkg: &pbsubstreams.Package{
				ProtoFiles: []*descriptorpb.FileDescriptorProto{
					{
						Name:    stringPtr("test.proto"),
						Package: stringPtr("test.package"),
						MessageType: []*descriptorpb.DescriptorProto{
							{
								Name: stringPtr("TestMessage"),
							},
						},
					},
				},
				Modules: &pbsubstreams.Modules{
					Modules: []*pbsubstreams.Module{
						{
							Name: "test_module",
							Output: &pbsubstreams.Module_Output{
								Type: "proto:test.package.NonExistentMessage",
							},
						},
					},
				},
			},
			expectError: true,
			errorMsg:    "proto message type \"test.package.NonExistentMessage\" not found",
		},
		{
			name: "module with nested proto message",
			pkg: &pbsubstreams.Package{
				ProtoFiles: []*descriptorpb.FileDescriptorProto{
					{
						Name:    stringPtr("test.proto"),
						Package: stringPtr("test.package"),
						MessageType: []*descriptorpb.DescriptorProto{
							{
								Name: stringPtr("OuterMessage"),
								NestedType: []*descriptorpb.DescriptorProto{
									{
										Name: stringPtr("InnerMessage"),
									},
								},
							},
						},
					},
				},
				Modules: &pbsubstreams.Modules{
					Modules: []*pbsubstreams.Module{
						{
							Name: "test_module",
							Output: &pbsubstreams.Module_Output{
								Type: "proto:test.package.OuterMessage.InnerMessage",
							},
						},
					},
				},
			},
			expectError: false,
		},
		{
			name: "multiple modules with mixed validity",
			pkg: &pbsubstreams.Package{
				ProtoFiles: []*descriptorpb.FileDescriptorProto{
					{
						Name:    stringPtr("test.proto"),
						Package: stringPtr("test.package"),
						MessageType: []*descriptorpb.DescriptorProto{
							{
								Name: stringPtr("ValidMessage"),
							},
						},
					},
				},
				Modules: &pbsubstreams.Modules{
					Modules: []*pbsubstreams.Module{
						{
							Name: "valid_module",
							Output: &pbsubstreams.Module_Output{
								Type: "proto:test.package.ValidMessage",
							},
						},
						{
							Name: "invalid_module",
							Output: &pbsubstreams.Module_Output{
								Type: "proto:test.package.InvalidMessage",
							},
						},
					},
				},
			},
			expectError: true,
			errorMsg:    "invalid_module",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateProtoOutputTypes(tt.pkg)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
					return
				}
				if tt.errorMsg != "" && !contains(err.Error(), tt.errorMsg) {
					t.Errorf("expected error to contain %q, got: %v", tt.errorMsg, err)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}

func TestBuildProtoTypeSet(t *testing.T) {
	tests := []struct {
		name       string
		protoFiles []*descriptorpb.FileDescriptorProto
		expected   map[string]bool
	}{
		{
			name:       "empty proto files",
			protoFiles: nil,
			expected:   map[string]bool{},
		},
		{
			name: "single message",
			protoFiles: []*descriptorpb.FileDescriptorProto{
				{
					Name:    stringPtr("test.proto"),
					Package: stringPtr("test.package"),
					MessageType: []*descriptorpb.DescriptorProto{
						{
							Name: stringPtr("TestMessage"),
						},
					},
				},
			},
			expected: map[string]bool{
				"test.package.TestMessage": true,
			},
		},
		{
			name: "nested messages",
			protoFiles: []*descriptorpb.FileDescriptorProto{
				{
					Name:    stringPtr("test.proto"),
					Package: stringPtr("test.package"),
					MessageType: []*descriptorpb.DescriptorProto{
						{
							Name: stringPtr("OuterMessage"),
							NestedType: []*descriptorpb.DescriptorProto{
								{
									Name: stringPtr("InnerMessage"),
									NestedType: []*descriptorpb.DescriptorProto{
										{
											Name: stringPtr("DeepMessage"),
										},
									},
								},
							},
						},
					},
				},
			},
			expected: map[string]bool{
				"test.package.OuterMessage":                          true,
				"test.package.OuterMessage.InnerMessage":             true,
				"test.package.OuterMessage.InnerMessage.DeepMessage": true,
			},
		},
		{
			name: "multiple files and packages",
			protoFiles: []*descriptorpb.FileDescriptorProto{
				{
					Name:    stringPtr("test1.proto"),
					Package: stringPtr("package1"),
					MessageType: []*descriptorpb.DescriptorProto{
						{
							Name: stringPtr("Message1"),
						},
					},
				},
				{
					Name:    stringPtr("test2.proto"),
					Package: stringPtr("package2"),
					MessageType: []*descriptorpb.DescriptorProto{
						{
							Name: stringPtr("Message2"),
						},
					},
				},
			},
			expected: map[string]bool{
				"package1.Message1": true,
				"package2.Message2": true,
			},
		},
		{
			name: "no package name",
			protoFiles: []*descriptorpb.FileDescriptorProto{
				{
					Name:    stringPtr("test.proto"),
					Package: nil,
					MessageType: []*descriptorpb.DescriptorProto{
						{
							Name: stringPtr("TestMessage"),
						},
					},
				},
			},
			expected: map[string]bool{
				"TestMessage": true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildProtoTypeSet(tt.protoFiles)

			if len(result) != len(tt.expected) {
				t.Errorf("expected %d types, got %d", len(tt.expected), len(result))
			}

			for expectedType := range tt.expected {
				if !result[expectedType] {
					t.Errorf("expected type %q not found in result", expectedType)
				}
			}

			for resultType := range result {
				if !tt.expected[resultType] {
					t.Errorf("unexpected type %q found in result", resultType)
				}
			}
		})
	}
}

// Helper functions
func stringPtr(s string) *string {
	return &s
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) &&
		(haystack == needle ||
			haystack[:len(needle)] == needle ||
			haystack[len(haystack)-len(needle):] == needle ||
			indexOf(haystack, needle) >= 0)
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

func TestValidateProtoOutputTypesIntegration(t *testing.T) {
	// Test with a realistic package structure
	pkg := &pbsubstreams.Package{
		ProtoFiles: []*descriptorpb.FileDescriptorProto{
			{
				Name:    stringPtr("events.proto"),
				Package: stringPtr("eth.erc20.v1"),
				MessageType: []*descriptorpb.DescriptorProto{
					{
						Name: stringPtr("Transfer"),
					},
					{
						Name: stringPtr("Approval"),
					},
				},
			},
			{
				Name:    stringPtr("blocks.proto"),
				Package: stringPtr("eth.block.v1"),
				MessageType: []*descriptorpb.DescriptorProto{
					{
						Name: stringPtr("Block"),
						NestedType: []*descriptorpb.DescriptorProto{
							{
								Name: stringPtr("Transaction"),
							},
						},
					},
				},
			},
		},
		Modules: &pbsubstreams.Modules{
			Modules: []*pbsubstreams.Module{
				{
					Name: "map_transfers",
					Output: &pbsubstreams.Module_Output{
						Type: "proto:eth.erc20.v1.Transfer",
					},
				},
				{
					Name: "map_approvals",
					Output: &pbsubstreams.Module_Output{
						Type: "proto:eth.erc20.v1.Approval",
					},
				},
				{
					Name: "map_blocks",
					Output: &pbsubstreams.Module_Output{
						Type: "proto:eth.block.v1.Block",
					},
				},
				{
					Name: "map_transactions",
					Output: &pbsubstreams.Module_Output{
						Type: "proto:eth.block.v1.Block.Transaction",
					},
				},
				{
					Name: "invalid_module",
					Output: &pbsubstreams.Module_Output{
						Type: "proto:eth.erc20.v1.NonExistent",
					},
				},
			},
		},
	}

	err := ValidateProtoOutputTypes(pkg)
	if err == nil {
		t.Fatal("expected validation error but got none")
	}

	expectedError := "module \"invalid_module\" has invalid proto output type \"proto:eth.erc20.v1.NonExistent\": proto message type \"eth.erc20.v1.NonExistent\" not found in package proto definitions"
	if err.Error() != expectedError {
		t.Errorf("expected error:\n%s\ngot:\n%s", expectedError, err.Error())
	}
}
