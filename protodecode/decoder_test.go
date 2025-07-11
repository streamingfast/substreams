package protodecode

import (
	"testing"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"google.golang.org/protobuf/types/descriptorpb"
)

func TestNewDecoder(t *testing.T) {
	// Create a minimal package for testing
	pkg := &pbsubstreams.Package{
		ProtoFiles: []*descriptorpb.FileDescriptorProto{},
		Modules: &pbsubstreams.Modules{
			Modules: []*pbsubstreams.Module{
				{
					Name: "test_module",
					Kind: &pbsubstreams.Module_KindMap_{
						KindMap: &pbsubstreams.Module_KindMap{
							OutputType: "proto:test.Message",
						},
					},
				},
			},
		},
	}

	outputStreamNames := []string{"test_module"}

	decoder, err := NewDecoder(pkg, outputStreamNames)
	if err != nil {
		t.Fatalf("NewDecoder failed: %v", err)
	}

	if decoder == nil {
		t.Fatal("Expected decoder to be non-nil")
	}

	// Test that the decoder has the expected message type
	if !decoder.HasMessageType("test_module") {
		t.Error("Expected decoder to have message type for test_module")
	}

	msgType := decoder.GetMessageType("test_module")
	if msgType != "test.Message" {
		t.Errorf("Expected message type 'test.Message', got '%s'", msgType)
	}
}

func TestOutputStreamPattern(t *testing.T) {
	tests := []struct {
		pattern string
		input   string
		matches bool
	}{
		{"exact_match", "exact_match", true},
		{"exact_match", "different", false},
		{"test_.*", "test_module", true},
		{"test_.*", "test_store", true},
		{"test_.*", "other_module", false},
		{"invalid_regex[", "anything", false}, // Invalid regex should fall back to exact match
	}

	for _, test := range tests {
		t.Run(test.pattern+"_"+test.input, func(t *testing.T) {
			pattern := NewOutputStreamPattern(test.pattern)
			result := pattern.Matches(test.input)
			if result != test.matches {
				t.Errorf("Pattern '%s' matching input '%s': expected %v, got %v",
					test.pattern, test.input, test.matches, result)
			}
		})
	}
}

func TestDecoder_HasMessageType(t *testing.T) {
	decoder := &Decoder{
		msgTypes: map[string]string{
			"module1": "type1",
			"module2": "type2",
		},
	}

	if !decoder.HasMessageType("module1") {
		t.Error("Expected HasMessageType to return true for module1")
	}

	if !decoder.HasMessageType("module2") {
		t.Error("Expected HasMessageType to return true for module2")
	}

	if decoder.HasMessageType("module3") {
		t.Error("Expected HasMessageType to return false for module3")
	}
}

func TestDecoder_GetMessageType(t *testing.T) {
	decoder := &Decoder{
		msgTypes: map[string]string{
			"module1": "type1",
			"module2": "type2",
		},
	}

	if msgType := decoder.GetMessageType("module1"); msgType != "type1" {
		t.Errorf("Expected GetMessageType to return 'type1', got '%s'", msgType)
	}

	if msgType := decoder.GetMessageType("nonexistent"); msgType != "" {
		t.Errorf("Expected GetMessageType to return empty string for nonexistent module, got '%s'", msgType)
	}
}
