// Package spool holds a from-proto sink's rows on local disk, pre-encoded into whatever
// the target database can take unchanged, and applies whole segments from a separate
// goroutine.
//
// The point is not raw throughput — pre-encoding is worth about 8% on its own, see
// sink/sql/db_proto/benchmarks. It is that the stream stops waiting for the database.
// Substreams throughput is paid for, so a slow or stalled database should cost disk, not
// download progress, and blocks already paid for should survive a restart rather than be
// streamed again.
//
// What the bytes on disk look like, and how a sealed segment reaches the server, are the
// driver's business: this package owns the segment lifecycle, the disk budget and the
// sizing, and delegates the rest through Codec and Applier.
package spool

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

	// Format says how the rows are laid out. Absent means FormatPGCopy, which is what
	// every segment written before formats existed carries.
	Format Format `json:"format,omitempty"`
	// LogFile and LogBytes describe the single interleaved file of FormatRowLog. The
	// table records still carry the column layout, since replaying a row needs it.
	LogFile  string `json:"log_file,omitempty"`
	LogBytes int64  `json:"log_bytes,omitempty"`
}

// BlockCount is how many blocks the segment covers. A segment carrying nothing but a
// cursor — every block in the flush produced no output — covers none.
func (m *Manifest) BlockCount() int64 {
	if m.FirstBlock == 0 || m.LastBlock < m.FirstBlock {
		return 0
	}

	return int64(m.LastBlock-m.FirstBlock) + 1
}

// RowCount is how many rows the segment holds across every table.
func (m *Manifest) RowCount() int64 {
	var rows int64
	for _, table := range m.Tables {
		rows += table.Rows
	}

	return rows
}

// CursorOnly reports a segment that carries no rows. It exists so that a stretch of
// blocks whose module output is empty still advances the cursor, rather than being
// streamed, and paid for, again on restart.
func (m *Manifest) CursorOnly() bool { return len(m.Tables) == 0 }

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

// WriteManifest is exported for a codec that needs to re-seal a segment.
func WriteManifest(dir string, manifest *Manifest) error {
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

// SanitizeFileName keeps a table name usable as a path component.
func SanitizeFileName(name string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_', r == '-':
			return r
		default:
			return '_'
		}
	}, name)
}
