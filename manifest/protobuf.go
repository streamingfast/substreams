package manifest

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"buf.build/gen/go/bufbuild/reflect/connectrpc/go/buf/reflect/v1beta1/reflectv1beta1connect"
	reflectv1beta1 "buf.build/gen/go/bufbuild/reflect/protocolbuffers/go/buf/reflect/v1beta1"
	"connectrpc.com/connect"
	"github.com/jhump/protoreflect/desc"
	"github.com/jhump/protoreflect/desc/protoparse"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/pb/system"
	sfproto "github.com/streamingfast/substreams/proto"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
)

func loadProtobufs(pkg *pbsubstreams.Package, manif *Manifest) ([]*desc.FileDescriptor, error) {

	seen := map[string]bool{}
	for _, file := range pkg.ProtoFiles {
		seen[*file.Name] = true
	}

	// System protos
	systemFiles, err := readSystemProtobufs()
	if err != nil {
		return nil, err
	}

	for _, file := range systemFiles.File {
		if _, found := seen[*file.Name]; found {
			continue
		}

		pkg.ProtoFiles = append(pkg.ProtoFiles, file)
		seen[*file.Name] = true
	}

	var importPaths []string
	for _, imp := range manif.Protobuf.ImportPaths {
		importPaths = append(importPaths, manif.resolvePath(imp))
	}

	// The manifest's root directory is always added to the list of import paths so that
	// files specified relative to the manifest's directory works properly. It is added last
	// so that if user's specified import paths contains the file, it's picked from their
	// import paths instead of the implicitly added folder.
	if manif.Workdir != "" {
		importPaths = append(importPaths, manif.Workdir)
	}

	// User-specified protos
	parser := &protoparse.Parser{
		ImportPaths:           importPaths,
		IncludeSourceCodeInfo: true,
		Accessor: func(filename string) (io.ReadCloser, error) {
			// This is a workaround for protoparse's parser that does not honor extensions (google.protobuf.FieldOptions) without access to the full source:
			// the source 'sf/substreams/options.proto' file is provided through go_embed, simulating that the file exists on disk.
			if strings.HasSuffix(filename, sfproto.OptionsPath) {
				return io.NopCloser(bytes.NewReader(sfproto.OptionsSource)), nil
			}
			return os.Open(filename)
		},
		LookupImportProto: func(file string) (*descriptorpb.FileDescriptorProto, error) {
			for _, protoFile := range pkg.ProtoFiles {
				if protoFile.GetName() == file {
					return protoFile, nil
				}
			}
			return nil, fmt.Errorf("proto file %q not found in package", file)
		},
	}

	for _, file := range manif.Protobuf.Files {
		if seen[file] {
			return nil, fmt.Errorf("WARNING: proto file %s already exists in system protobufs, do not include it in your manifest", file)
		}
	}

	customFiles, err := parser.ParseFiles(manif.Protobuf.Files...)
	if err != nil {
		return nil, fmt.Errorf("error parsing proto files %q (import paths: %q): %w", manif.Protobuf.Files, importPaths, err)
	}
	for _, fd := range customFiles {
		pkg.ProtoFiles = append(pkg.ProtoFiles, fd.AsFileDescriptorProto())
	}

	return customFiles, nil
}

func loadDescriptorSets(pkg *pbsubstreams.Package, manif *Manifest) ([]*desc.FileDescriptor, error) {
	var out []*desc.FileDescriptor
	for _, url := range manif.Protobuf.DescriptorSets {

		authToken := os.Getenv("BUFBUILD_AUTH_TOKEN")
		if authToken == "" {
			return nil, fmt.Errorf("missing BUFBUILD_AUTH_TOKEN")
		}

		client := reflectv1beta1connect.NewFileDescriptorSetServiceClient(
			http.DefaultClient,
			"https://buf.build",
		)

		request := connect.NewRequest(&reflectv1beta1.GetFileDescriptorSetRequest{
			Module: url,
		})

		request.Header().Set("Authorization", "Bearer "+authToken)
		fileDescriptorSet, err := client.GetFileDescriptorSet(context.Background(), request)
		if err != nil {
			return nil, fmt.Errorf("getting file descriptor set for %s: %w", url, err)
		}

		fd, err := desc.CreateFileDescriptorFromSet(fileDescriptorSet.Msg.FileDescriptorSet)
		if err != nil {
			return nil, fmt.Errorf("creating file descriptor from set for %s: %w", url, err)
		}
		out = append(out, fd)
	}

	for _, fd := range out {
		pkg.ProtoFiles = append(pkg.ProtoFiles, fd.AsFileDescriptorProto())
	}

	return out, nil
}

func readSystemProtobufs() (*descriptorpb.FileDescriptorSet, error) {
	fds := &descriptorpb.FileDescriptorSet{}
	err := proto.Unmarshal(system.ProtobufDescriptors, fds)
	if err != nil {
		return nil, err
	}

	return fds, nil
}
