// Package buffer holds a from-proto sink's rows on local disk, in the exact byte
// format `COPY ... FROM STDIN (FORMAT BINARY)` expects, and applies them to PostgreSQL
// from a separate goroutine.
//
// The point is not raw throughput — pre-encoding to disk is worth about 8% on its own,
// see sink/sql/db_proto/benchmarks. It is that the stream stops waiting for the
// database. Substreams throughput is paid for, so a slow or stalled PostgreSQL should
// cost disk, not download progress, and blocks already paid for should survive a restart
// rather than be streamed again.
package buffer

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/streamingfast/substreams/sink/sql/db_proto/sql/postgres/pgcopy"
)

const manifestFileName = "manifest.json"

// Manifest is a segment's commit record. Its presence, parseable and sealed, is what
// makes a segment eligible to be applied; anything else is a torn write from a crash.
type Manifest struct {
	FirstBlock uint64        `json:"first_block"`
	LastBlock  uint64        `json:"last_block"`
	Cursor     string        `json:"cursor"`
	Tables     []TableRecord `json:"tables"`
	Sealed     bool          `json:"sealed"`
}

// TableRecord describes one table's PGCOPY file inside a segment.
//
// Schema and Name are the identifiers the server actually stored, not the logical table
// name the walk uses: the dialect writes DDL unquoted, so a message called BalanceChange
// lives in the relation balancechange, and a COPY has to say so.
type TableRecord struct {
	Name     string   `json:"name"`
	Schema   string   `json:"schema"`
	Relation string   `json:"relation"`
	File     string   `json:"file"`
	Columns  []string `json:"columns"`
	Rows     int64    `json:"rows"`
	Bytes    int64    `json:"bytes"`
}

// segment is one unit of work: a directory of PGCOPY files plus the manifest that makes
// them applicable.
type segment struct {
	dir        string
	firstBlock uint64
	lastBlock  uint64
	cursor     string
	tables     map[string]*tableFile
	bytes      int64
}

// tableFile is one table's open PGCOPY stream within a segment.
type tableFile struct {
	name   string
	target *pgcopy.Table
	path   string
	file   *os.File
	writer *pgcopy.Writer
}

func newSegment(dir string, firstBlock uint64) (*segment, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("creating segment directory %s: %w", dir, err)
	}

	return &segment{dir: dir, firstBlock: firstBlock, tables: map[string]*tableFile{}}, nil
}

// writeRow appends one row to the table's stream, opening it on first use.
func (s *segment) writeRow(table string, into *pgcopy.Table, values []any) error {
	target, ok := s.tables[table]
	if !ok {
		path := filepath.Join(s.dir, sanitizeFileName(table)+".pgcopy")

		file, err := os.Create(path)
		if err != nil {
			return fmt.Errorf("creating %s: %w", path, err)
		}

		writer, err := pgcopy.NewWriter(file, into.Columns)
		if err != nil {
			file.Close()
			return fmt.Errorf("starting pgcopy stream for %q: %w", table, err)
		}

		target = &tableFile{name: table, target: into, path: path, file: file, writer: writer}
		s.tables[table] = target
	}

	if err := pgcopy.NormalizeRow(target.target.Columns, values); err != nil {
		return fmt.Errorf("normalizing a row of %q: %w", table, err)
	}

	return target.writer.WriteRow(values)
}

// seal closes every stream and writes the manifest last, so a segment is either fully
// described or visibly incomplete.
//
// Only the manifest is fsynced. A process crash is fully covered by that; a machine
// crash may leave a data file short, which recovery catches by checking each file's
// length against the manifest.
func (s *segment) seal() (*Manifest, error) {
	manifest := &Manifest{
		FirstBlock: s.firstBlock,
		LastBlock:  s.lastBlock,
		Cursor:     s.cursor,
		Sealed:     true,
	}

	for _, name := range slices.Sorted(maps.Keys(s.tables)) {
		target := s.tables[name]

		if err := target.writer.Close(); err != nil {
			return nil, fmt.Errorf("closing pgcopy stream for %q: %w", name, err)
		}

		info, err := target.file.Stat()
		if err != nil {
			return nil, fmt.Errorf("sizing %s: %w", target.path, err)
		}
		if err := target.file.Close(); err != nil {
			return nil, fmt.Errorf("closing %s: %w", target.path, err)
		}

		columns := make([]string, len(target.target.Columns))
		for i, column := range target.target.Columns {
			columns[i] = column.Name
		}

		manifest.Tables = append(manifest.Tables, TableRecord{
			Name:     name,
			Schema:   target.target.Schema,
			Relation: target.target.Name,
			File:     filepath.Base(target.path),
			Columns:  columns,
			Rows:     target.writer.Rows(),
			Bytes:    info.Size(),
		})
	}

	if err := writeManifest(s.dir, manifest); err != nil {
		return nil, err
	}

	return manifest, nil
}

// pendingBytes is how much this segment has written so far, from the writers' own
// counters rather than a stat() per call.
func (s *segment) pendingBytes() int64 {
	var total int64
	for _, target := range s.tables {
		total += target.writer.Bytes()
	}

	return total
}

// blockCount is how many blocks this segment covers so far.
func (s *segment) blockCount() int64 {
	if s.firstBlock == 0 || s.lastBlock < s.firstBlock {
		return 0
	}

	return int64(s.lastBlock-s.firstBlock) + 1
}

// discard closes and removes a segment that will never be applied.
func (s *segment) discard() {
	for _, target := range s.tables {
		target.file.Close()
	}
	os.RemoveAll(s.dir)
}

func writeManifest(dir string, manifest *Manifest) error {
	encoded, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding manifest: %w", err)
	}

	path := filepath.Join(dir, manifestFileName)
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("creating %s: %w", path, err)
	}
	defer file.Close()

	if _, err := file.Write(encoded); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}

	// The manifest is the commit record, so it is the one thing worth the fsync.
	return file.Sync()
}

func readManifest(dir string) (*Manifest, error) {
	encoded, err := os.ReadFile(filepath.Join(dir, manifestFileName))
	if err != nil {
		return nil, err
	}

	var manifest Manifest
	if err := json.Unmarshal(encoded, &manifest); err != nil {
		return nil, fmt.Errorf("decoding manifest in %s: %w", dir, err)
	}

	return &manifest, nil
}

// sanitizeFileName keeps a table name usable as a path component.
func sanitizeFileName(name string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_', r == '-':
			return r
		default:
			return '_'
		}
	}, name)
}
