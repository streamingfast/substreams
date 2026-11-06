package benchmarks

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
)

// A frame file is a sequence of uint32-length-prefixed byte blobs. It lets prebuilt
// SQL statements be stored on disk and streamed back without any parsing, so a variant
// reading them pays no cost the real implementation would not.

type frameWriter struct {
	out    *bufio.Writer
	header []byte
}

func newFrameWriter(w io.Writer) *frameWriter {
	return &frameWriter{out: bufio.NewWriterSize(w, 1024*1024), header: make([]byte, 4)}
}

func (w *frameWriter) write(payload []byte) error {
	binary.BigEndian.PutUint32(w.header, uint32(len(payload)))
	if _, err := w.out.Write(w.header); err != nil {
		return err
	}
	_, err := w.out.Write(payload)

	return err
}

func (w *frameWriter) flush() error { return w.out.Flush() }

type frameReader struct {
	in     *bufio.Reader
	header []byte
	buf    []byte
}

func newFrameReader(r io.Reader) *frameReader {
	return &frameReader{in: bufio.NewReaderSize(r, 1024*1024), header: make([]byte, 4)}
}

// next returns the next frame, or io.EOF. The returned slice is only valid until the
// following call.
func (r *frameReader) next() ([]byte, error) {
	if _, err := io.ReadFull(r.in, r.header); err != nil {
		return nil, err
	}

	size := int(binary.BigEndian.Uint32(r.header))
	if cap(r.buf) < size {
		r.buf = make([]byte, size)
	}
	r.buf = r.buf[:size]

	if _, err := io.ReadFull(r.in, r.buf); err != nil {
		return nil, fmt.Errorf("reading frame payload of %d bytes: %w", size, err)
	}

	return r.buf, nil
}
