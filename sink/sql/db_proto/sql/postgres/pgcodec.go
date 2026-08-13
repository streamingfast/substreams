package postgres

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/streamingfast/substreams/sink/sql/bytes"
	"github.com/streamingfast/substreams/sink/sql/db_proto/sql/postgres/pgcopy"
	"github.com/streamingfast/substreams/sink/sql/db_proto/sql/spool"
)

// pgCodec lays a segment out the way the chosen write mode can send it unchanged.
//
// Three formats, one per mode: binary COPY files, rendered SQL tuples per table, and a
// single interleaved log in walk order. The rendering the accumulator used to do at flush
// time happens here instead, off the database's critical path.
type pgCodec struct {
	format   spool.Format
	tables   map[string]*pgcopy.Table
	encoding bytes.Encoding
}

func newPGCodec(format spool.Format, tables map[string]*pgcopy.Table, encoding bytes.Encoding) *pgCodec {
	return &pgCodec{format: format, tables: tables, encoding: encoding}
}

func (c *pgCodec) Format() spool.Format { return c.format }

func (c *pgCodec) OpenSegment(dir string) (spool.SegmentWriter, error) {
	return &pgSegment{dir: dir, codec: c, tables: map[string]*pgTableFile{}}, nil
}

// Verify checks each data file against what the manifest recorded. The rendered formats
// are length-framed, so the size check is the whole of it. Binary COPY is not, which is
// why it also gets its trailer checked.
func (c *pgCodec) Verify(dir string, manifest *spool.Manifest) error {
	if manifest.Format == spool.FormatRowLog {
		if manifest.LogFile == "" {
			return nil
		}

		return verifySize(filepath.Join(dir, manifest.LogFile), manifest.LogBytes)
	}

	for _, table := range manifest.Tables {
		path := filepath.Join(dir, table.File)

		if err := verifySize(path, table.Bytes); err != nil {
			return err
		}

		if manifest.Format == spool.FormatPGCopy {
			if err := verifyTrailer(path); err != nil {
				return err
			}
		}
	}

	return nil
}

func verifySize(path string, expected int64) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("%s is missing: %w", filepath.Base(path), err)
	}
	if info.Size() != expected {
		return fmt.Errorf("%s is %d bytes, the manifest recorded %d", filepath.Base(path), info.Size(), expected)
	}

	return nil
}

// verifyTrailer checks the two bytes that terminate a binary COPY stream.
func verifyTrailer(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return err
	}
	if info.Size() < int64(pgcopy.HeaderSize+pgcopy.TrailerSize) {
		return fmt.Errorf("%s is too short to be a pgcopy stream", filepath.Base(path))
	}

	trailer := make([]byte, pgcopy.TrailerSize)
	if _, err := file.ReadAt(trailer, info.Size()-int64(pgcopy.TrailerSize)); err != nil {
		return fmt.Errorf("reading the trailer of %s: %w", filepath.Base(path), err)
	}
	if trailer[0] != 0xFF || trailer[1] != 0xFF {
		return fmt.Errorf("%s does not end with a pgcopy trailer", filepath.Base(path))
	}

	return nil
}

// rowWriter is one table's open stream, whichever format it is in.
type rowWriter interface {
	WriteRow(values []any) error
	Rows() int64
	Bytes() int64
	Close() error
}

// pgTableFile is one table's stream within a segment. Under FormatRowLog there is no
// per-table stream and writer stays nil: the record carries only the column layout a
// replay needs, and the row counter.
type pgTableFile struct {
	target *pgcopy.Table
	path   string
	file   *os.File
	writer rowWriter
	rows   int64
}

type pgSegment struct {
	dir    string
	codec  *pgCodec
	tables map[string]*pgTableFile

	// log is the single interleaved file of FormatRowLog, shared by every table.
	log     *spool.FrameWriter
	logPath string
	values  []string
}

func (s *pgSegment) WriteRow(table string, values []any) error {
	target, ok := s.tables[table]
	if !ok {
		layout, known := s.codec.tables[table]
		if !known {
			return fmt.Errorf("no column layout known for table %q", table)
		}

		created, err := s.openTable(table, layout)
		if err != nil {
			return err
		}
		target = created
	}

	if s.codec.format == spool.FormatPGCopy {
		// Binary COPY does no coercion, so the values have to match the column types the
		// server reported exactly. This is value normalization, including materializing
		// the configured protobuf-bytes representation for text columns; it is not SQL
		// rendering. The rendered formats go through the dialect instead.
		if err := pgcopy.NormalizeRowWithEncoding(target.target.Columns, values, s.codec.encoding); err != nil {
			return fmt.Errorf("normalizing a row of %q: %w", table, err)
		}
	}

	target.rows++

	if s.codec.format == spool.FormatRowLog {
		tuple := s.renderTuple(values)

		return s.log.WriteRecord(table, tuple)
	}

	return target.writer.WriteRow(values)
}

func (s *pgSegment) renderTuple(values []any) string {
	s.values = s.values[:0]
	for _, value := range values {
		s.values = append(s.values, s.codec.render(value))
	}

	return strings.Join(s.values, ",")
}

func (c *pgCodec) render(value any) string { return ValueToString(value, c.encoding) }

func (s *pgSegment) openTable(table string, layout *pgcopy.Table) (*pgTableFile, error) {
	target := &pgTableFile{target: layout}

	switch s.codec.format {
	case spool.FormatRowLog:
		if s.log == nil {
			s.logPath = filepath.Join(s.dir, "rows.log")
			file, err := os.Create(s.logPath)
			if err != nil {
				return nil, fmt.Errorf("creating %s: %w", s.logPath, err)
			}
			s.log = spool.NewFrameWriter(file)
		}

	case spool.FormatTuples:
		path := filepath.Join(s.dir, spool.SanitizeFileName(table)+".tuples")
		file, err := os.Create(path)
		if err != nil {
			return nil, fmt.Errorf("creating %s: %w", path, err)
		}
		target.path, target.file = path, file
		target.writer = &pgTupleWriter{frames: spool.NewFrameWriter(file), segment: s}

	default:
		path := filepath.Join(s.dir, spool.SanitizeFileName(table)+".pgcopy")
		file, err := os.Create(path)
		if err != nil {
			return nil, fmt.Errorf("creating %s: %w", path, err)
		}

		writer, err := pgcopy.NewWriter(file, layout.Columns)
		if err != nil {
			file.Close()
			return nil, fmt.Errorf("starting pgcopy stream for %q: %w", table, err)
		}
		target.path, target.file, target.writer = path, file, writer
	}

	s.tables[table] = target

	return target, nil
}

func (s *pgSegment) PendingBytes() int64 {
	if s.log != nil {
		return s.log.Bytes()
	}

	var total int64
	for _, target := range s.tables {
		if target.writer != nil {
			total += target.writer.Bytes()
		}
	}

	return total
}

func (s *pgSegment) Seal(manifest *spool.Manifest) error {
	if s.log != nil {
		if err := s.log.Close(); err != nil {
			return fmt.Errorf("closing %s: %w", s.logPath, err)
		}

		info, err := os.Stat(s.logPath)
		if err != nil {
			return fmt.Errorf("sizing %s: %w", s.logPath, err)
		}
		manifest.LogFile = filepath.Base(s.logPath)
		manifest.LogBytes = info.Size()
	}

	for _, name := range slices.Sorted(maps.Keys(s.tables)) {
		target := s.tables[name]

		record := spool.TableRecord{
			Name:     name,
			Schema:   target.target.Schema,
			Relation: target.target.Name,
			Rows:     target.rows,
		}

		record.Columns = make([]string, len(target.target.Columns))
		for i, column := range target.target.Columns {
			record.Columns[i] = column.Name
		}

		if target.writer != nil {
			if err := target.writer.Close(); err != nil {
				return fmt.Errorf("closing the stream of %q: %w", name, err)
			}

			info, err := os.Stat(target.path)
			if err != nil {
				return fmt.Errorf("sizing %s: %w", target.path, err)
			}
			record.File = filepath.Base(target.path)
			record.Bytes = info.Size()
		}

		manifest.Tables = append(manifest.Tables, record)
	}

	return nil
}

func (s *pgSegment) Discard() {
	for _, target := range s.tables {
		if target.file != nil {
			target.file.Close()
		}
	}
	if s.log != nil {
		s.log.Close()
	}
	os.RemoveAll(s.dir)
}

// pgTupleWriter renders a row into one framed record.
type pgTupleWriter struct {
	frames  *spool.FrameWriter
	segment *pgSegment
}

func (w *pgTupleWriter) WriteRow(values []any) error {
	return w.frames.WriteRecord(w.segment.renderTuple(values))
}

func (w *pgTupleWriter) Rows() int64  { return w.frames.Rows() }
func (w *pgTupleWriter) Bytes() int64 { return w.frames.Bytes() }
func (w *pgTupleWriter) Close() error { return w.frames.Close() }
