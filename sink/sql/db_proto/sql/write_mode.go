package sql

import "fmt"

// WriteMode says how a sealed spool segment reaches the database.
//
// It is deliberately the operator's choice rather than something derived from a directory
// flag and the shape of the schema's foreign keys, which is how the sink used to decide.
// A mode that the driver or the schema cannot support is an error, not a silent downgrade
// to something an order of magnitude slower.
type WriteMode string

const (
	// WriteModeAuto picks copy on PostgreSQL, batch-insert on ClickHouse, and row-insert
	// for a schema whose foreign keys cannot be ordered.
	WriteModeAuto WriteMode = "auto"

	// WriteModeCopy loads each table file with `COPY ... FROM STDIN (FORMAT BINARY)`.
	// Measured at ~7x the multi-row INSERT path, see sink/sql/db_proto/benchmarks.
	WriteModeCopy WriteMode = "copy"

	// WriteModeBatchInsert builds one multi-row INSERT per table, split to stay under the
	// driver's bind-parameter and statement-size limits.
	WriteModeBatchInsert WriteMode = "batch-insert"

	// WriteModeRowInsert issues one prepared INSERT per row, in walk order. It is the
	// fallback for a schema whose foreign keys form a cycle: a cycle has no table order,
	// so grouping rows by table cannot keep a parent ahead of its children, while the walk
	// itself always does.
	WriteModeRowInsert WriteMode = "row-insert"
)

func ParseWriteMode(in string) (WriteMode, error) {
	switch WriteMode(in) {
	case "", WriteModeAuto:
		return WriteModeAuto, nil
	case WriteModeCopy:
		return WriteModeCopy, nil
	case WriteModeBatchInsert:
		return WriteModeBatchInsert, nil
	case WriteModeRowInsert:
		return WriteModeRowInsert, nil
	}

	return "", fmt.Errorf("invalid write mode %q, expected one of %q, %q, %q or %q",
		in, WriteModeAuto, WriteModeCopy, WriteModeBatchInsert, WriteModeRowInsert)
}
