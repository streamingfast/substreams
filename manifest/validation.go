package manifest

import (
	"fmt"
	"strings"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"google.golang.org/protobuf/types/descriptorpb"
)

// ValidateProtoOutputTypes validates that all modules with proto:... output types
// have corresponding protobuf definitions in the package.
func ValidateProtoOutputTypes(pkg *pbsubstreams.Package) error {
	if pkg.Modules == nil {
		return nil
	}

	// Build a set of available proto message types from the package's proto files
	availableTypes := buildProtoTypeSet(pkg.ProtoFiles)

	for _, module := range pkg.Modules.Modules {
		if module.Output == nil {
			continue
		}

		outputType := module.Output.Type
		if !strings.HasPrefix(outputType, "proto:") {
			continue
		}

		protoTypeName := strings.TrimPrefix(outputType, "proto:")

		if !availableTypes[protoTypeName] {
			return fmt.Errorf("module %q has invalid proto output type %q: proto message type %q not found in package proto definitions", module.Name, outputType, protoTypeName)
		}
	}

	return nil
}

// buildProtoTypeSet creates a set of all available proto message types from file descriptors.
// Returns a map where keys are fully qualified message type names and values are always true.
func buildProtoTypeSet(protoFiles []*descriptorpb.FileDescriptorProto) map[string]bool {
	typeSet := make(map[string]bool)

	for _, fileDesc := range protoFiles {
		if fileDesc == nil {
			continue
		}

		// Get package prefix
		packageName := ""
		if fileDesc.Package != nil {
			packageName = *fileDesc.Package + "."
		}

		for _, msgDesc := range fileDesc.MessageType {
			if msgDesc.Name != nil {
				fullName := packageName + *msgDesc.Name
				typeSet[fullName] = true
				addNestedMessageTypes(typeSet, packageName+*msgDesc.Name, msgDesc)
			}
		}
	}

	return typeSet
}

// addNestedMessageTypes recursively adds nested message types to the type set.
//
// This helper function processes nested message definitions within a parent message
// and adds them to the type set with their fully qualified names. It handles arbitrary
// levels of nesting by recursively processing each nested message's own nested types.
//
// Parameters:
// - typeSet: The map to add discovered message types to
// - parentName: The fully qualified name of the parent message
// - msgDesc: The message descriptor containing potential nested message types
//
// For a nested message structure like:
//
//	message Outer {
//	  message Inner {
//	    message Deep {}
//	  }
//	}
//
// This function would add:
// - "package.Outer.Inner"
// - "package.Outer.Inner.Deep"
func addNestedMessageTypes(typeSet map[string]bool, parentName string, msgDesc *descriptorpb.DescriptorProto) {
	for _, nestedMsg := range msgDesc.NestedType {
		if nestedMsg.Name != nil {
			fullName := parentName + "." + *nestedMsg.Name
			typeSet[fullName] = true

			// Recursively add further nested types
			addNestedMessageTypes(typeSet, fullName, nestedMsg)
		}
	}
}
