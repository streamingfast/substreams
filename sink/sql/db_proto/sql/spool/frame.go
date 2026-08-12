package spool

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"os"
)

// recordSize bounds a single framed record, so a corrupt length cannot make recovery
// allocate an arbitrary amount before the length check has a chance to reject the file.
const recordSize = 64 << 20

// FrameWriter writes length-prefixed records.
//
// Framing rather than lines because a rendered tuple carries SQL literals, and a text
// column holding a newline would end a line in the middle of a row. The length check that
// recovery does against the manifest then covers a torn write, the same way the binary
// COPY trailer does for FormatPGCopy.
type FrameWriter struct {
	file    *os.File
	writer  *bufio.Writer
	rows    int64
	written int64
	header  [4]byte
}

func NewFrameWriter(file *os.File) *FrameWriter {
	return &FrameWriter{file: file, writer: bufio.NewWriterSize(file, 1<<20)}
}

// WriteRecord appends one record made of the given fields, each length-prefixed.
func (w *FrameWriter) WriteRecord(fields ...string) error {
	for _, field := range fields {
		binary.BigEndian.PutUint32(w.header[:], uint32(len(field)))
		if _, err := w.writer.Write(w.header[:]); err != nil {
			return err
		}
		if _, err := w.writer.WriteString(field); err != nil {
			return err
		}
		w.written += int64(len(w.header)) + int64(len(field))
	}
	w.rows++

	return nil
}

func (w *FrameWriter) Rows() int64  { return w.rows }
func (w *FrameWriter) Bytes() int64 { return w.written }

func (w *FrameWriter) Close() error {
	if err := w.writer.Flush(); err != nil {
		return err
	}

	return w.file.Close()
}

// FrameReader reads back what FrameWriter produced.
type FrameReader struct {
	file   *os.File
	reader *bufio.Reader
	header [4]byte
}

func OpenFrameReader(path string) (*FrameReader, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}

	return &FrameReader{file: file, reader: bufio.NewReaderSize(file, 1<<20)}, nil
}

func (r *FrameReader) Close() error { return r.file.Close() }

// ReadField returns the next field, or io.EOF once the file is exhausted.
func (r *FrameReader) ReadField() (string, error) {
	if _, err := io.ReadFull(r.reader, r.header[:]); err != nil {
		return "", err
	}

	size := binary.BigEndian.Uint32(r.header[:])
	if size > recordSize {
		return "", fmt.Errorf("record of %d bytes exceeds the %d byte limit", size, recordSize)
	}

	field := make([]byte, size)
	if _, err := io.ReadFull(r.reader, field); err != nil {
		return "", fmt.Errorf("reading a %d byte record: %w", size, err)
	}

	return string(field), nil
}
