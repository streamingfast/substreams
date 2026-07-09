package proto

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/jhump/protoreflect/desc"
	v1 "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"google.golang.org/protobuf/types/descriptorpb"
)

func FileDescriptorForOutputType(spkg *v1.Package, err error, deps map[string]*desc.FileDescriptor, outputType string) (*desc.FileDescriptor, error) {
	for _, p := range spkg.ProtoFiles {
		fd, err := desc.CreateFileDescriptor(p, slices.Collect(maps.Values(deps))...)
		if err != nil {
			return nil, fmt.Errorf("creating file descriptor: %w", err)
		}

		for _, md := range fd.GetMessageTypes() {
			if md.GetFullyQualifiedName() == outputType {
				return fd, nil
			}
		}
	}

	return nil, fmt.Errorf("could not find file descriptor")
}

func ModuleOutputType(spkg *v1.Package, moduleName string) string {
	outputType := ""
	for _, m := range spkg.Modules.Modules {
		if m.Name == moduleName {
			outputType = strings.TrimPrefix(m.Output.Type, "proto:")
			break
		}
	}
	return outputType
}
func ResolveDependencies(protoFiles map[string]*descriptorpb.FileDescriptorProto) (map[string]*desc.FileDescriptor, error) {
	out := map[string]*desc.FileDescriptor{}
	for _, protoFile := range protoFiles {
		err := resolveDependencies(protoFile, protoFiles, out)
		if err != nil {
			return nil, fmt.Errorf("error resolving dependencies: %w", err)
		}
	}

	return out, nil
}

func resolveDependencies(protoFile *descriptorpb.FileDescriptorProto, protoFiles map[string]*descriptorpb.FileDescriptorProto, deps map[string]*desc.FileDescriptor) error {
	if deps[protoFile.GetName()] != nil {
		return nil
	}
	if len(protoFile.Dependency) != 0 {
		for _, dep := range protoFile.Dependency {
			depProtoFile, found := protoFiles[dep]
			if !found {
				return fmt.Errorf("could not find proto file for dependency %q", dep)
			}
			err := resolveDependencies(depProtoFile, protoFiles, deps)
			if err != nil {
				return fmt.Errorf("error resolving dependencies: %w", err)
			}
		}
	}

	d, err := desc.CreateFileDescriptor(protoFile, slices.Collect(maps.Values(deps))...)
	if err != nil {
		return fmt.Errorf("creating file descriptor: %w", err)
	}

	deps[protoFile.GetName()] = d
	return nil
}
