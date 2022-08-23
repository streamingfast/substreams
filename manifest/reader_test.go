package manifest

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jhump/protoreflect/desc/protoparse"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/descriptorpb"
)

func TestReader_Read(t *testing.T) {

	systemProtoDefs := readSystemProtoDescriptors(t)
	absolutePathToDep2, err := filepath.Abs("testdata/dep2.yaml")
	require.NoError(t, err)

	absolutePathToProto2, err := filepath.Abs("testdata/proto2")
	require.NoError(t, err)

	spkg1Content, err := os.ReadFile("testdata/spkg1/spkg1-v0.0.0.spkg")
	require.NoError(t, err)

	remoteServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// FIXME: Handle more "spkgX" path if required
		w.Write(spkg1Content)
	}))
	defer remoteServer.Close()

	tests := []struct {
		name      string
		env       map[string]string
		want      *pbsubstreams.Package
		assertion require.ErrorAssertionFunc
	}{
		{
			"testdata/bare_minimum.yaml",
			nil,
			&pbsubstreams.Package{
				Version:    1,
				ProtoFiles: append(systemProtoDefs),
				Modules:    &pbsubstreams.Modules{},
				PackageMeta: []*pbsubstreams.PackageMetadata{
					{
						Name:    "test",
						Version: "v0.0.0",
					},
				},
			},
			require.NoError,
		},
		{
			"testdata/imports_relative_path.yaml",
			nil,
			&pbsubstreams.Package{
				Version:    1,
				ProtoFiles: append(systemProtoDefs),
				Modules:    &pbsubstreams.Modules{},
				PackageMeta: []*pbsubstreams.PackageMetadata{
					{
						Name:    "test",
						Version: "v0.0.0",
					},
					{
						Name:    "dep1",
						Version: "v0.0.0",
					},
				},
			},
			require.NoError,
		},
		{
			"testdata/imports_http_url.yaml",
			map[string]string{
				"SERVER_HOST": strings.Replace(remoteServer.URL, "http://", "", 1),
			},
			&pbsubstreams.Package{
				Version:    1,
				ProtoFiles: append(systemProtoDefs),
				Modules:    &pbsubstreams.Modules{},
				PackageMeta: []*pbsubstreams.PackageMetadata{
					{
						Name:    "test",
						Version: "v0.0.0",
					},
					{
						Name:    "spkg1",
						Version: "v0.0.0",
					},
				},
			},
			require.NoError,
		},
		{
			"testdata/imports_expand_env_variables.yaml",
			map[string]string{
				"RELATIVE_PATH_TO_DEP1": "./dep1.yaml",
				"ABSOLUTE_PATH_TO_DEP2": absolutePathToDep2,
			},
			&pbsubstreams.Package{
				Version:    1,
				ProtoFiles: append(systemProtoDefs),
				Modules:    &pbsubstreams.Modules{},
				PackageMeta: []*pbsubstreams.PackageMetadata{
					{
						Name:    "test",
						Version: "v0.0.0",
					},
					{
						Name:    "dep1",
						Version: "v0.0.0",
					},
					{
						Name:    "dep2",
						Version: "v0.0.0",
					},
				},
			},
			require.NoError,
		},
		{
			"testdata/protobuf_importPaths_relative_path.yaml",
			nil,
			&pbsubstreams.Package{
				Version: 1,
				ProtoFiles: append(
					systemProtoDefs,
					readProtoDescriptor(t, "testdata/proto1", "sf/substreams/test1.proto"),
				),
				Modules: &pbsubstreams.Modules{},
				PackageMeta: []*pbsubstreams.PackageMetadata{
					{
						Name:    "test",
						Version: "v0.0.0",
					},
				},
			},
			require.NoError,
		},
		{
			"testdata/protobuf_importPaths_expand_variables.yaml",
			map[string]string{
				"RELATIVE_PATH_TO_PROTO1": "./proto1",
				"ABSOLUTE_PATH_TO_PROTO2": absolutePathToProto2,
			},
			&pbsubstreams.Package{
				Version: 1,
				ProtoFiles: append(
					systemProtoDefs,
					readProtoDescriptor(t, "testdata/proto1", "sf/substreams/test1.proto"),
					readProtoDescriptor(t, "testdata/proto2", "sf/substreams/test2.proto"),
				),
				Modules: &pbsubstreams.Modules{},
				PackageMeta: []*pbsubstreams.PackageMetadata{
					{
						Name:    "test",
						Version: "v0.0.0",
					},
				},
			},
			require.NoError,
		},
	}
	for _, tt := range tests {
		t.Run(strings.Replace(tt.name, "testdata/", "", 1), func(t *testing.T) {
			manifestPath, err := filepath.Abs(tt.name)
			require.NoError(t, err)

			for envKey, envValue := range tt.env {
				t.Setenv(envKey, envValue)
			}

			r := NewReader(manifestPath, SkipSourceCodeReader())
			got, err := r.Read()
			tt.assertion(t, err)
			assertProtoEqual(t, tt.want, got)
		})
	}
}

func readProtoDescriptor(t *testing.T, importPath string, file string) (out *descriptorpb.FileDescriptorProto) {
	t.Helper()

	parser := protoparse.Parser{
		ImportPaths:           []string{importPath},
		IncludeSourceCodeInfo: true,
	}

	customFiles, err := parser.ParseFiles(file)
	require.NoError(t, err)
	require.Len(t, customFiles, 1)

	return customFiles[0].AsFileDescriptorProto()
}

func readSystemProtoDescriptors(t *testing.T) (out []*descriptorpb.FileDescriptorProto) {
	t.Helper()

	systemProtoFiles, err := readSystemProtobufs()
	require.NoError(t, err)

	return systemProtoFiles.File
}
