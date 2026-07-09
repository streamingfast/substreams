package writer

import (
	"context"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"

	"github.com/streamingfast/bstream"
	"github.com/streamingfast/dstore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewBufferedIO(t *testing.T) {
	listFiles := func(root string) (out []string) {
		filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err == nil {
				if !d.IsDir() {
					out = append(out, strings.Replace(path, root, "", 1))
				}
			}

			return err
		})
		return
	}

	newSimplerWritter := func(t *testing.T, writer *BufferedIO) *simplerWriter {
		return &simplerWriter{
			writer: writer,
			t:      t,
		}
	}

	tests := []struct {
		name       string
		bufferSize uint64
		checks     func(t *testing.T, writer *BufferedIO, workingDir string, output *dstore.MockStore)
	}{
		{
			"all in memory no write",
			16,
			func(t *testing.T, writer *BufferedIO, workingDir string, output *dstore.MockStore) {
				simpler := newSimplerWritter(t, writer)

				require.NoError(t, writer.StartBoundary(bstream.NewInclusiveRange(0, 10)))
				require.NoError(t, simpler.Write([]byte("{first}")))
				require.NoError(t, simpler.Write([]byte("{second}")))

				uploadeable, err := writer.CloseBoundary(context.Background())
				require.NoError(t, err)
				writtenFiles := listFiles(workingDir)

				_, err = uploadeable.Upload(context.Background(), output)
				require.NoError(t, err)

				assert.Len(t, writtenFiles, 0)
				assert.Equal(t, map[string][]byte{
					"0000000000-0000000009": []byte(`{first}{second}`),
				}, output.Files)
			},
		},

		{
			"write to file",
			4,
			func(t *testing.T, writer *BufferedIO, workingDir string, output *dstore.MockStore) {
				simpler := newSimplerWritter(t, writer)

				require.NoError(t, writer.StartBoundary(bstream.NewInclusiveRange(0, 10)), "start boundary")
				require.NoError(t, simpler.Write([]byte("{first}")), "write first content")
				require.NoError(t, simpler.Write([]byte("{second}")), "write second content")

				//require.NoError(t, writer.CloseBoundary(context.Background()), "closing boundary")
				uploadeable, err := writer.CloseBoundary(context.Background())
				require.NoError(t, err)

				writtenFiles := listFiles(workingDir)

				_, err = uploadeable.Upload(context.Background(), output)
				require.NoError(t, err, "upload file")

				assert.ElementsMatch(t, []string{"/0000000000-0000000010.tmp.jsonl"}, writtenFiles)
				assert.Equal(t, map[string][]byte{
					"0000000000-0000000009": []byte(`{first}{second}`),
				}, output.Files)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			workingDir := t.TempDir()
			outputStore := dstore.NewMockStore(nil)

			writer := NewBufferedIO(tt.bufferSize, workingDir, FileTypeJSONL, zlog)

			tt.checks(t, writer, workingDir, outputStore)
		})
	}
}

type simplerWriter struct {
	writer *BufferedIO
	t      *testing.T
}

func (w *simplerWriter) Write(buf []byte) (err error) {
	w.t.Helper()

	n, err := w.writer.Write(buf)
	if n < 0 {
		w.t.Fatal("writer returned negative byte written, this invalid according to 'io.Writer' spec")
	}

	if n > len(buf) {
		w.t.Fatal("writer returned more written byte than our actual buffer length, this invalid according to 'io.Writer' spec")
	}

	if n < len(buf) && err == nil {
		w.t.Fatal("writer returned less written byte than our actual buffer length but err is nil, this invalid according to 'io.Writer' spec")
	}

	return err
}
