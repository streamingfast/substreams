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
	"go.uber.org/zap"
	"golang.org/x/mod/semver"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
)

type DescriptorCache struct {
	cacheDir       string
	memoryCache    map[string]*descriptorpb.FileDescriptorSet
	useMemoryCache bool
}

func newDescriptorCache() *DescriptorCache {
	configDir, err := os.UserConfigDir()
	if err != nil {
		zlog.Debug("failed to get user config directory, falling back to in-memory cache", zap.Error(err))
		return &DescriptorCache{
			memoryCache:    make(map[string]*descriptorpb.FileDescriptorSet),
			useMemoryCache: true,
		}
	}

	cacheDir := filepath.Join(configDir, "substreams", "buf-cache")
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		zlog.Debug("failed to create cache directory, falling back to in-memory cache", zap.Error(err))
		return &DescriptorCache{
			memoryCache:    make(map[string]*descriptorpb.FileDescriptorSet),
			useMemoryCache: true,
		}
	}

	return &DescriptorCache{cacheDir: cacheDir}
}

func (c *DescriptorCache) isDeterministicVersion(version string) bool {
	// Cache valid semver versions and buf commit references (both are immutable)
	return semver.IsValid(version) || isValidBufCommitRef(version)
}

func (c *DescriptorCache) generateCacheKey(module, version string, symbols []string) string {
	h := sha256.New()
	h.Write([]byte(module))
	h.Write([]byte(version))
	// symbols contains fully-qualified protobuf type names (eg, sf.substreams.v1.Package)
	for _, symbol := range symbols {
		h.Write([]byte(symbol))
	}
	return hex.EncodeToString(h.Sum(nil))
}

func (c *DescriptorCache) cacheFilePath(cacheKey string) string {
	return filepath.Join(c.cacheDir, cacheKey+".file_descriptor.binpb")
}

func (c *DescriptorCache) load(cacheKey string) (*descriptorpb.FileDescriptorSet, error) {
	if c.useMemoryCache {
		if fds, ok := c.memoryCache[cacheKey]; ok {
			return fds, nil
		}
		return nil, fmt.Errorf("cache miss")
	}

	data, err := os.ReadFile(c.cacheFilePath(cacheKey))
	if err != nil {
		return nil, err
	}

	fds := &descriptorpb.FileDescriptorSet{}
	err = proto.Unmarshal(data, fds)
	return fds, err
}

func (c *DescriptorCache) save(cacheKey string, fds *descriptorpb.FileDescriptorSet) error {
	if c.useMemoryCache {
		c.memoryCache[cacheKey] = fds
		return nil
	}

	data, err := proto.Marshal(fds)
	if err != nil {
		return err
	}

	return os.WriteFile(c.cacheFilePath(cacheKey), data, 0644)
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

func loadDescriptorSets(ctx context.Context, pkg *pbsubstreams.Package, manif *Manifest) ([]*desc.FileDescriptor, error) {
	seen := map[string]bool{}
	for _, file := range pkg.ProtoFiles {
		seen[*file.Name] = true
	}

	// Initialize cache once for all descriptor sets
	cache := newDescriptorCache()

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

		// Try to load from cache first for deterministic versions
		if cache.isDeterministicVersion(descriptor.Version) {
			cacheKey := cache.generateCacheKey(descriptor.Module, descriptor.Version, descriptor.Symbols)
			cachedFds, err := cache.load(cacheKey)
			if err == nil {
				fileDescriptorSetResponse = cachedFds
			} else {
				zlog.Debug("cache miss for descriptor set", zap.String("module", descriptor.Module), zap.String("version", descriptor.Version))
			}
		}

		// Fetch from buf.build if not cached or cache disabled
		if fileDescriptorSetResponse == nil {
			request := connect.NewRequest(&reflectv1beta1.GetFileDescriptorSetRequest{
				Module:  descriptor.Module,
				Symbols: descriptor.Symbols,
				Version: descriptor.Version,
			})

			authToken := os.Getenv("BUFBUILD_AUTH_TOKEN")
			if authToken != "" {
				request.Header().Set("Authorization", "Bearer "+authToken)
			}

			fileDescriptorSet, err := client.GetFileDescriptorSet(ctx, request)
			if err != nil {
				return nil, fmt.Errorf("getting file descriptor set for %s: %w", descriptor.Module, err)
			}

			fileDescriptorSetResponse = fileDescriptorSet.Msg.FileDescriptorSet

			// Save to cache for deterministic versions
			if cache.isDeterministicVersion(descriptor.Version) {
				cacheKey := cache.generateCacheKey(descriptor.Module, descriptor.Version, descriptor.Symbols)
				if err := cache.save(cacheKey, fileDescriptorSetResponse); err != nil {
					zlog.Debug("failed to save descriptor set to cache", zap.Error(err))
				}
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
