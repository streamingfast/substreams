package spool

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"os"
)

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
		// The prefix is four bytes, so a longer field would be written with a truncated
		// length and read back as a shorter one — a segment that looks intact and is not.
		if int64(len(field)) > math.MaxUint32 {
			return fmt.Errorf("a field of %d bytes cannot be framed, the length prefix holds at most %d", len(field), int64(math.MaxUint32))
		}

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

	// remaining is what is left of the file, and is what bounds a record's declared
	// length. A fixed ceiling here used to reject rows the writer had accepted, which no
	// restart could get past: the segment verified, failed to apply, and was replayed
	// again on the next start.
	remaining int64
}

func OpenFrameReader(path string) (*FrameReader, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}

	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, fmt.Errorf("sizing %s: %w", path, err)
	}

	return &FrameReader{file: file, reader: bufio.NewReaderSize(file, 1<<20), remaining: info.Size()}, nil
}

func (r *FrameReader) Close() error { return r.file.Close() }

// ReadField returns the next field, or io.EOF once the file is exhausted.
func (r *FrameReader) ReadField() (string, error) {
	if _, err := io.ReadFull(r.reader, r.header[:]); err != nil {
		return "", err
	}

	r.remaining -= int64(len(r.header))

	// A record cannot be longer than what is left of the file holding it. That is what
	// catches a corrupt length before it allocates, and it is the whole of the bound: a
	// record the writer produced always fits, and the row it came from was already held
	// whole in memory to be written.
	size := binary.BigEndian.Uint32(r.header[:])
	if int64(size) > r.remaining {
		return "", fmt.Errorf("a record of %d bytes does not fit in the %d bytes left of the file", size, r.remaining)
	}

	field := make([]byte, size)
	if _, err := io.ReadFull(r.reader, field); err != nil {
		return "", fmt.Errorf("reading a %d byte record: %w", size, err)
	}
	r.remaining -= int64(size)

	return string(field), nil
}
