package manifest

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"buf.build/gen/go/bufbuild/reflect/connectrpc/go/buf/reflect/v1beta1/reflectv1beta1connect"
	reflectv1beta1 "buf.build/gen/go/bufbuild/reflect/protocolbuffers/go/buf/reflect/v1beta1"
	"connectrpc.com/connect"
	"github.com/jhump/protoreflect/desc"
	"github.com/jhump/protoreflect/desc/protoparse"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/pb/system"
	sfproto "github.com/streamingfast/substreams/proto"
	"golang.org/x/mod/semver"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
)

type DescriptorCache struct {
	cacheDir string
}

func newDescriptorCache() (*DescriptorCache, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return nil, fmt.Errorf("getting user config directory: %w", err)
	}

	cacheDir := filepath.Join(configDir, "substreams", "buf-cache")
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return nil, fmt.Errorf("creating cache directory: %w", err)
	}

	return &DescriptorCache{cacheDir: cacheDir}, nil
}

func (c *DescriptorCache) shouldCache(version string) bool {
	// Only cache valid semver versions
	return semver.IsValid(version)
}

func (c *DescriptorCache) generateCacheKey(module, version string, symbols []string) string {
	h := sha256.New()
	h.Write([]byte(module))
	h.Write([]byte(version))
	for _, symbol := range symbols {
		h.Write([]byte(symbol))
	}
	return hex.EncodeToString(h.Sum(nil))
}

func (c *DescriptorCache) load(cacheKey string) (*descriptorpb.FileDescriptorSet, error) {
	cacheFile := filepath.Join(c.cacheDir, cacheKey+".file_descriptor.binpb")
	data, err := os.ReadFile(cacheFile)
	if err != nil {
		return nil, err
	}

	fds := &descriptorpb.FileDescriptorSet{}
	err = proto.Unmarshal(data, fds)
	return fds, err
}

func (c *DescriptorCache) save(cacheKey string, fds *descriptorpb.FileDescriptorSet) error {
	data, err := proto.Marshal(fds)
	if err != nil {
		return err
	}

	cacheFile := filepath.Join(c.cacheDir, cacheKey+".file_descriptor.binpb")
	return os.WriteFile(cacheFile, data, 0644)
}

func loadLocalProtobufs(pkg *pbsubstreams.Package, manif *Manifest) ([]*desc.FileDescriptor, error) {

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
	seen := map[string]bool{}
	for _, file := range pkg.ProtoFiles {
		seen[*file.Name] = true
	}

	// Initialize cache once for all descriptor sets
	cache, err := newDescriptorCache()
	if err != nil {
		return nil, fmt.Errorf("initializing descriptor cache: %w", err)
	}

	var out []*desc.FileDescriptor
	var outProto []*descriptorpb.FileDescriptorProto
	for _, descriptor := range manif.Protobuf.DescriptorSets {

		if descriptor.LocalPath != "" {
			f, err := os.Open(descriptor.LocalPath)
			if err != nil {
				return nil, fmt.Errorf("error opening local protobuf descriptor file %q: %w", descriptor.LocalPath, err)
			}
			defer f.Close()

			b, err := io.ReadAll(f)
			if err != nil {
				return nil, fmt.Errorf("error reading local protobuf descriptor file %q: %w", descriptor.LocalPath, err)
			}

			protoDescContainer := &pbsubstreams.Package{}
			proto.Unmarshal(b, protoDescContainer)

			for _, fdProto := range protoDescContainer.ProtoFiles {
				if _, found := seen[fdProto.GetName()]; found {
					continue
				}
				seen[fdProto.GetName()] = true
				outProto = append(outProto, fdProto)
			}
			continue
		}

		client := reflectv1beta1connect.NewFileDescriptorSetServiceClient(
			http.DefaultClient,
			"https://buf.build",
		)

		var fileDescriptorSetResponse *descriptorpb.FileDescriptorSet

		// Don't use cache for mutable versions or empty versions
		if !cache.shouldCache(descriptor.Version) {
			request := connect.NewRequest(&reflectv1beta1.GetFileDescriptorSetRequest{
				Module:  descriptor.Module,
				Symbols: descriptor.Symbols,
				Version: descriptor.Version,
			})

			authToken := os.Getenv("BUFBUILD_AUTH_TOKEN")
			if authToken != "" {
				request.Header().Set("Authorization", "Bearer "+authToken)
			}

			fileDescriptorSet, err := client.GetFileDescriptorSet(context.Background(), request)
			if err != nil {
				return nil, fmt.Errorf("getting file descriptor set for %s: %w", descriptor.Module, err)
			}

			fileDescriptorSetResponse = fileDescriptorSet.Msg.FileDescriptorSet
		} else {
			// Try to load from cache first for valid semver versions
			cacheKey := cache.generateCacheKey(descriptor.Module, descriptor.Version, descriptor.Symbols)
			cachedFds, err := cache.load(cacheKey)

			if err == nil {
				// Cache hit
				fileDescriptorSetResponse = cachedFds
			} else {
				// Cache miss, fetch from buf.build
				request := connect.NewRequest(&reflectv1beta1.GetFileDescriptorSetRequest{
					Module:  descriptor.Module,
					Symbols: descriptor.Symbols,
					Version: descriptor.Version,
				})

				authToken := os.Getenv("BUFBUILD_AUTH_TOKEN")
				if authToken != "" {
					request.Header().Set("Authorization", "Bearer "+authToken)
				}

				fileDescriptorSet, err := client.GetFileDescriptorSet(context.Background(), request)
				if err != nil {
					return nil, fmt.Errorf("getting file descriptor set for %s: %w", descriptor.Module, err)
				}

				fileDescriptorSetResponse = fileDescriptorSet.Msg.FileDescriptorSet

				// Save to cache (ignore errors to not break the build)
				_ = cache.save(cacheKey, fileDescriptorSetResponse)
			}
		}

		fdMap, err := desc.CreateFileDescriptorsFromSet(fileDescriptorSetResponse)
		if err != nil {
			return nil, fmt.Errorf("creating file descriptors from set: %w", err)
		}

		for _, fd := range fdMap {
			if _, found := seen[fd.GetName()]; found {
				continue
			}
			seen[fd.GetName()] = true
			out = append(out, fd)
		}
		// TEMP FIX FOR TEST DEPLOYMENT (keeping this here still until we've decided)
		// time.Sleep(1020 * time.Millisecond)
	}

	for _, fd := range out {
		pkg.ProtoFiles = append(pkg.ProtoFiles, fd.AsFileDescriptorProto())
	}
	pkg.ProtoFiles = append(pkg.ProtoFiles, outProto...)

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

func loadProtobufFromDirectory(pkg *pbsubstreams.Package, protoPath string) ([]*desc.FileDescriptor, error) {
	seen := map[string]bool{}
	for _, file := range pkg.ProtoFiles {
		seen[*file.Name] = true
	}

	parser := &protoparse.Parser{
		ImportPaths:           []string{protoPath},
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

	// Find all .proto files in the directory
	var protoFiles []string
	err := filepath.Walk(protoPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasSuffix(path, ".proto") {
			// Get relative path from protoPath
			relPath, err := filepath.Rel(protoPath, path)
			if err != nil {
				return err
			}
			protoFiles = append(protoFiles, relPath)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("error walking proto directory %q: %w", protoPath, err)
	}

	if len(protoFiles) == 0 {
		return nil, nil // No proto files found, return empty
	}

	// Filter out files that already exist
	var filesToParse []string
	for _, file := range protoFiles {
		if !seen[file] {
			filesToParse = append(filesToParse, file)
		}
	}

	if len(filesToParse) == 0 {
		return nil, nil // All files already exist
	}

	customFiles, err := parser.ParseFiles(filesToParse...)
	if err != nil {
		return nil, fmt.Errorf("error parsing proto files from directory %q: %w", protoPath, err)
	}

	for _, fd := range customFiles {
		pkg.ProtoFiles = append(pkg.ProtoFiles, fd.AsFileDescriptorProto())
	}

	return customFiles, nil
}

func loadProtobufFromDescriptorSet(pkg *pbsubstreams.Package, descriptorSetPath string) ([]*desc.FileDescriptor, error) {
	seen := map[string]bool{}
	for _, file := range pkg.ProtoFiles {
		seen[*file.Name] = true
	}

	f, err := os.Open(descriptorSetPath)
	if err != nil {
		return nil, fmt.Errorf("error opening protobuf descriptor set file %q: %w", descriptorSetPath, err)
	}
	defer f.Close()

	b, err := io.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("error reading protobuf descriptor set file %q: %w", descriptorSetPath, err)
	}

	// Try to unmarshal as FileDescriptorSet first
	fds := &descriptorpb.FileDescriptorSet{}
	err = proto.Unmarshal(b, fds)
	if err != nil {
		// If that fails, try to unmarshal as Package (which contains FileDescriptorProto slice)
		protoDescContainer := &pbsubstreams.Package{}
		err = proto.Unmarshal(b, protoDescContainer)
		if err != nil {
			return nil, fmt.Errorf("error unmarshaling protobuf descriptor set file %q: not a valid FileDescriptorSet or Package: %w", descriptorSetPath, err)
		}

		// Add FileDescriptorProtos from Package
		for _, fdProto := range protoDescContainer.ProtoFiles {
			if _, found := seen[fdProto.GetName()]; !found {
				seen[fdProto.GetName()] = true
				pkg.ProtoFiles = append(pkg.ProtoFiles, fdProto)
			}
		}
		return nil, nil // No FileDescriptors to return, just added to pkg.ProtoFiles
	}

	// Create FileDescriptors from FileDescriptorSet
	fdMap, err := desc.CreateFileDescriptorsFromSet(fds)
	if err != nil {
		return nil, fmt.Errorf("creating file descriptors from set %q: %w", descriptorSetPath, err)
	}

	var out []*desc.FileDescriptor
	for _, fd := range fdMap {
		if _, found := seen[fd.GetName()]; !found {
			seen[fd.GetName()] = true
			out = append(out, fd)
			pkg.ProtoFiles = append(pkg.ProtoFiles, fd.AsFileDescriptorProto())
		}
	}

	return out, nil
}
