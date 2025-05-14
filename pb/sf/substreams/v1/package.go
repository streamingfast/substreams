package pbsubstreams

import (
	"fmt"
	"strings"

	protoV1 "github.com/golang/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"
)

func (x *Package) GetProtoFileDescriptorSet() *descriptorpb.FileDescriptorSet {
	set := &descriptorpb.FileDescriptorSet{}
	set.File = x.GetProtoFiles()

	return set
}

func (x *Package) GetProtoFilesRegistry() (*protoregistry.Files, error) {
	return protodesc.NewFiles(x.GetProtoFileDescriptorSet())
}

func (x *Package) NewAnyResolver() (*PackageAnyResolver, error) {
	files, err := x.GetProtoFilesRegistry()
	if err != nil {
		return nil, fmt.Errorf("get proto files registry: %w", err)
	}

	return &PackageAnyResolver{
		registry: files,
	}, nil
}

// PackageAnyResolver is an implementation of the AnyResolver interface
// from jsonpb.AnyResolver package.
type PackageAnyResolver struct {
	registry *protoregistry.Files
}

// FindMessageDescriptor takes a msgType string, which is a type URL, either fully qualified
// with "type.googleapis.com/" or just the message full name and returns
// the message descriptor.
//
// If the msgType is not found or of the wrong type, it returns an error.
func (w *PackageAnyResolver) FindMessageDescriptor(msgType string) (protoreflect.MessageDescriptor, error) {
	sanitizedTypeURL := strings.TrimPrefix(msgType, "type.googleapis.com/")

	desc, err := w.registry.FindDescriptorByName(protoreflect.FullName(sanitizedTypeURL))
	if err != nil {
		return nil, fmt.Errorf("find descriptor %s by name: %w", sanitizedTypeURL, err)
	}

	if desc == nil {
		return nil, fmt.Errorf("descriptor %s not found", sanitizedTypeURL)
	}

	msgDesc, ok := desc.(protoreflect.MessageDescriptor)
	if !ok {
		return nil, fmt.Errorf("descriptor %s found but of type %T but expected it to be of type protoreflect.MessageDescriptor", sanitizedTypeURL, desc)
	}

	return msgDesc, nil
}

// Resolve takes a msgType string, which is a type URL, either fully qualified
// with "type.googleapis.com/" or just the message full name and returns
// a new instance of the message type.
//
// If the msgType is not found or of the wrong type, it returns an error.
func (w *PackageAnyResolver) Resolve(msgType string) (protoV1.Message, error) {
	msgDesc, err := w.FindMessageDescriptor(msgType)
	if err != nil {
		return nil, err
	}

	return protoV1.MessageV1(dynamicpb.NewMessage(msgDesc)), nil
}
