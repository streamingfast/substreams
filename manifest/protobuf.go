package manifest

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/jhump/protoreflect/desc"

	"github.com/jhump/protoreflect/desc/protoparse"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/pb/system"
	sfproto "github.com/streamingfast/substreams/proto"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
)

func loadProtobufs(pkg *pbsubstreams.Package, manif *Manifest) ([]*desc.FileDescriptor, error) {
	// System protos
	systemFiles, err := readSystemProtobufs()
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for _, file := range systemFiles.File {
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

func readSystemProtobufs() (*descriptorpb.FileDescriptorSet, error) {
	fds := &descriptorpb.FileDescriptorSet{}
	err := proto.Unmarshal(system.ProtobufDescriptors, fds)
	if err != nil {
		return nil, err
	}

	return fds, nil
}
