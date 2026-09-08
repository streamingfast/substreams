// Package pgcopy writes the PostgreSQL binary COPY format ("PGCOPY").
//
// The point of writing this format directly is that a file holding it can be
// handed to `COPY ... FROM STDIN (FORMAT BINARY)` as raw bytes: the flush becomes
// an io.Copy from file to socket, with no re-encoding, no escaping and no parsing.
//
// Binary COPY performs no type coercion whatsoever: the bytes for a column must
// match that column's type OID exactly or the server aborts the COPY. Always
// resolve OIDs from the live catalog with [LoadColumns] rather than deriving them
// from a declared type name.
package pgcopy

import (
	"bufio"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"reflect"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// signature is the fixed 11-byte PGCOPY header signature.
var signature = []byte{'P', 'G', 'C', 'O', 'P', 'Y', '\n', 0xFF, '\r', '\n', 0x00}

// HeaderSize is the size of the file header: 11-byte signature, flags and header
// extension length.
const HeaderSize = 11 + 4 + 4

// TrailerSize is the size of the file trailer, an int16 holding -1.
const TrailerSize = 2

// Column is a target column with the type OID the server expects for it.
type Column struct {
	Name string
	OID  uint32
}

// Querier is the subset of pgx used to read the catalog, satisfied by *pgx.Conn,
// *pgxpool.Pool and pgx.Tx.
type Querier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

// LoadColumns returns the columns of schema.table in attribute order, with the
// type OID the server will require during a binary COPY.
func LoadColumns(ctx context.Context, q Querier, schema, table string) ([]Column, error) {
	resolved, err := ResolveTable(ctx, q, pgx.Identifier{schema, table}.Sanitize())
	if err != nil {
		return nil, err
	}

	return resolved.Columns, nil
}

// Table is a table as the server actually holds it: the identifier it stored, and the
// column layout a binary COPY has to match.
type Table struct {
	Schema  string
	Name    string
	Columns []Column
}

// ResolveTable looks a table up by the same reference the dialect writes into its DDL,
// letting the server apply its own parsing rules.
//
// This matters because the from-proto dialect emits table names unquoted, so a message
// called BalanceChange becomes the relation balancechange. Querying pg_class for the
// name as written finds nothing, and a COPY that quotes it as written targets a relation
// that does not exist. Going through to_regclass and reading the stored name back avoids
// having to reimplement identifier folding.
func ResolveTable(ctx context.Context, q Querier, reference string) (*Table, error) {
	const query = `
		SELECT n.nspname, c.relname, a.attname, a.atttypid
		FROM pg_attribute a
		JOIN pg_class c ON c.oid = a.attrelid
		JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE a.attrelid = to_regclass($1) AND a.attnum > 0 AND NOT a.attisdropped
		ORDER BY a.attnum`

	rows, err := q.Query(ctx, query, reference)
	if err != nil {
		return nil, fmt.Errorf("querying columns of %s: %w", reference, err)
	}
	defer rows.Close()

	out := &Table{}
	for rows.Next() {
		var col Column
		if err := rows.Scan(&out.Schema, &out.Name, &col.Name, &col.OID); err != nil {
			return nil, fmt.Errorf("scanning column of %s: %w", reference, err)
		}
		out.Columns = append(out.Columns, col)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating columns of %s: %w", reference, err)
	}
	if len(out.Columns) == 0 {
		return nil, fmt.Errorf("table %s has no columns, does it exist?", reference)
	}

	return out, nil
}

// CopySQL builds the statement to feed a stream written by [Writer] into the server.
func CopySQL(schema, table string, cols []Column) string {
	names := make([]string, len(cols))
	for i, col := range cols {
		names[i] = pgx.Identifier{col.Name}.Sanitize()
	}

	return fmt.Sprintf("COPY %s (%s) FROM STDIN (FORMAT BINARY)",
		pgx.Identifier{schema, table}.Sanitize(),
		joinComma(names),
	)
}

func joinComma(in []string) string {
	out := ""
	for i, s := range in {
		if i > 0 {
			out += ", "
		}
		out += s
	}
	return out
}

// Writer encodes rows into the binary COPY format. It is not safe for concurrent use.
type Writer struct {
	out     *bufio.Writer
	cols    []Column
	types   *pgtype.Map
	scratch []byte
	rows    int64
	bytes   int64
	closed  bool

	// Map.Encode resolves an encode plan on every call, which dominates the per-value
	// cost once the column type is known. Rows in a COPY stream are homogeneous, so the
	// plan is resolved once per column and reused, re-resolving only if a later row
	// presents a different Go type for that column.
	plans     []pgtype.EncodePlan
	planTypes []reflect.Type
}

// NewWriter wraps w and writes the file header. The caller must call [Writer.Close]
// to emit the trailer, without which the server rejects the stream.
func NewWriter(w io.Writer, cols []Column) (*Writer, error) {
	if len(cols) == 0 {
		return nil, fmt.Errorf("at least one column is required")
	}
	if len(cols) > 1<<15-1 {
		return nil, fmt.Errorf("too many columns: %d", len(cols))
	}

	buffered := bufio.NewWriterSize(w, 256*1024)

	header := make([]byte, 0, HeaderSize)
	header = append(header, signature...)
	header = binary.BigEndian.AppendUint32(header, 0) // flags: no OIDs
	header = binary.BigEndian.AppendUint32(header, 0) // header extension area length
	if _, err := buffered.Write(header); err != nil {
		return nil, fmt.Errorf("writing header: %w", err)
	}

	return &Writer{
		out:       buffered,
		cols:      cols,
		types:     pgtype.NewMap(),
		scratch:   make([]byte, 0, 4096),
		plans:     make([]pgtype.EncodePlan, len(cols)),
		planTypes: make([]reflect.Type, len(cols)),
	}, nil
}

// WriteRow encodes one tuple. values must be positional against the columns given
// to [NewWriter]; a nil value is written as SQL NULL.
func (w *Writer) WriteRow(values []any) error {
	if w.closed {
		return fmt.Errorf("writer is closed")
	}
	if len(values) != len(w.cols) {
		return fmt.Errorf("expected %d values, got %d", len(w.cols), len(values))
	}

	w.scratch = binary.BigEndian.AppendUint16(w.scratch[:0], uint16(len(w.cols)))

	for i, value := range values {
		lengthAt := len(w.scratch)
		w.scratch = binary.BigEndian.AppendUint32(w.scratch, 0)

		if value == nil {
			binary.BigEndian.PutUint32(w.scratch[lengthAt:], 0xFFFFFFFF)
			continue
		}

		plan, err := w.planFor(i, value)
		if err != nil {
			return err
		}

		encoded, err := plan.Encode(value, w.scratch)
		if err != nil {
			return fmt.Errorf("encoding column %q (oid %d) from %T: %w", w.cols[i].Name, w.cols[i].OID, value, err)
		}

		if encoded == nil {
			// SQL NULL is a length of -1 and no payload.
			binary.BigEndian.PutUint32(w.scratch[lengthAt:], 0xFFFFFFFF)
			continue
		}

		w.scratch = encoded
		binary.BigEndian.PutUint32(w.scratch[lengthAt:], uint32(len(w.scratch)-lengthAt-4))
	}

	if _, err := w.out.Write(w.scratch); err != nil {
		return fmt.Errorf("writing tuple: %w", err)
	}
	w.rows++
	w.bytes += int64(len(w.scratch))

	return nil
}

// planFor returns the cached encode plan for a column, resolving it on first use and
// whenever the Go type presented for that column changes.
func (w *Writer) planFor(column int, value any) (pgtype.EncodePlan, error) {
	valueType := reflect.TypeOf(value)
	if w.plans[column] != nil && w.planTypes[column] == valueType {
		return w.plans[column], nil
	}

	plan := w.types.PlanEncode(w.cols[column].OID, pgtype.BinaryFormatCode, value)
	if plan == nil {
		return nil, fmt.Errorf("no binary encoder for column %q (oid %d) from %T",
			w.cols[column].Name, w.cols[column].OID, value)
	}

	w.plans[column] = plan
	w.planTypes[column] = valueType

	return plan, nil
}

// Rows returns how many tuples have been written so far.
func (w *Writer) Rows() int64 { return w.rows }

// Bytes returns how many tuple bytes have been written, excluding header and trailer.
// It is a counter rather than a file size so a caller can poll it cheaply.
func (w *Writer) Bytes() int64 { return w.bytes }

// Close writes the trailer and flushes. It does not close the underlying writer.
func (w *Writer) Close() error {
	if w.closed {
		return nil
	}
	w.closed = true

	if _, err := w.out.Write([]byte{0xFF, 0xFF}); err != nil {
		return fmt.Errorf("writing trailer: %w", err)
	}
	if err := w.out.Flush(); err != nil {
		return fmt.Errorf("flushing: %w", err)
	}

	return nil
}
