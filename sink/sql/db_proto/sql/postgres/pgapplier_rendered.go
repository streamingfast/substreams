package postgres

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/streamingfast/substreams/sink/sql/db_proto/sql/spool"
)

// maxStatementBytes caps one generated INSERT. The values are rendered literals rather
// than bind parameters, so the 65535 parameter limit does not apply and the real ceiling
// is what the server will parse in one go. A few megabytes keeps the parse cost per row
// negligible without building a statement the server has to hold whole.
const maxStatementBytes = 4 << 20

// applyTuples loads a FormatTuples segment: one multi-row INSERT per table, chunked, in
// the same foreign key order the COPY path uses.
func (a *pgApplier) applyTuples(ctx context.Context, tx pgx.Tx, dir string, manifest *spool.Manifest) error {
	for _, table := range a.copyOrder(manifest.Tables) {
		if table.File == "" {
			continue
		}

		if err := a.insertTupleFile(ctx, tx, filepath.Join(dir, table.File), table); err != nil {
			return err
		}
	}

	return nil
}

func (a *pgApplier) insertTupleFile(ctx context.Context, tx pgx.Tx, path string, table spool.TableRecord) error {
	reader, err := spool.OpenFrameReader(path)
	if err != nil {
		return fmt.Errorf("opening %s: %w", path, err)
	}
	defer reader.Close()

	prefix := insertPrefix(table)

	var (
		statement strings.Builder
		batched   int
		applied   int64
	)
	flush := func() error {
		if batched == 0 {
			return nil
		}
		if _, err := tx.Exec(ctx, statement.String()); err != nil {
			return fmt.Errorf("inserting %d rows into %q: %w", batched, table.Name, err)
		}
		statement.Reset()
		batched = 0

		return nil
	}

	for {
		tuple, err := reader.ReadField()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("reading %s: %w", path, err)
		}

		if batched > 0 && statement.Len()+len(tuple)+3 > maxStatementBytes {
			if err := flush(); err != nil {
				return err
			}
		}
		if batched == 0 {
			statement.WriteString(prefix)
		} else {
			statement.WriteString(",")
		}
		statement.WriteString("(")
		statement.WriteString(tuple)
		statement.WriteString(")")

		batched++
		applied++
	}

	if err := flush(); err != nil {
		return err
	}

	if applied != table.Rows {
		return fmt.Errorf("inserted %d rows into %q but the manifest recorded %d", applied, table.Name, table.Rows)
	}

	return nil
}

// applyRowLog loads a FormatRowLog segment, replaying the walk's own order.
//
// Rows are not grouped by table here, and that is the point: this format exists for a
// schema whose foreign keys form a cycle, where no table order can keep a parent ahead of
// its children. The walk always does, so its order is what gets replayed — one statement
// per row, which is what makes it the slowest mode and the fallback rather than a choice.
func (a *pgApplier) applyRowLog(ctx context.Context, tx pgx.Tx, dir string, manifest *spool.Manifest) error {
	if manifest.LogFile == "" {
		return nil
	}

	path := filepath.Join(dir, manifest.LogFile)
	reader, err := spool.OpenFrameReader(path)
	if err != nil {
		return fmt.Errorf("opening %s: %w", path, err)
	}
	defer reader.Close()

	prefixes := make(map[string]string, len(manifest.Tables))
	expected := make(map[string]int64, len(manifest.Tables))
	for _, table := range manifest.Tables {
		prefixes[table.Name] = insertPrefix(table)
		expected[table.Name] = table.Rows
	}

	applied := make(map[string]int64, len(manifest.Tables))
	for {
		table, err := reader.ReadField()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("reading %s: %w", path, err)
		}

		tuple, err := reader.ReadField()
		if err != nil {
			return fmt.Errorf("reading the row of %q in %s: %w", table, path, err)
		}

		prefix, found := prefixes[table]
		if !found {
			return fmt.Errorf("%s holds a row of %q, which the manifest does not describe", path, table)
		}

		if _, err := tx.Exec(ctx, prefix+"("+tuple+")"); err != nil {
			return fmt.Errorf("inserting a row into %q: %w", table, err)
		}
		applied[table]++
	}

	for name, want := range expected {
		if applied[name] != want {
			return fmt.Errorf("inserted %d rows into %q but the manifest recorded %d", applied[name], name, want)
		}
	}

	return nil
}

// insertPrefix builds `INSERT INTO schema.relation (cols) VALUES `, quoting through the
// identifiers the server itself reported so the statement matches what was created.
func insertPrefix(table spool.TableRecord) string {
	columns := make([]string, len(table.Columns))
	for i, name := range table.Columns {
		columns[i] = pgx.Identifier{name}.Sanitize()
	}

	return fmt.Sprintf("INSERT INTO %s (%s) VALUES ",
		pgx.Identifier{table.Schema, table.Relation}.Sanitize(),
		strings.Join(columns, ", "))
}
