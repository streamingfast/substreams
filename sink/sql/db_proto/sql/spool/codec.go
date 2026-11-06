package spool

import "context"

// Format names an on-disk layout. It is stored in the manifest so a segment written by an
// earlier run is read back the way it was written.
type Format string

const (
	// FormatPGCopy is one file per table in PostgreSQL's binary COPY wire format. It is
	// also what a segment written before formats existed carries, hence the zero value.
	FormatPGCopy Format = ""

	// FormatTuples is one file per table of rendered SQL value tuples, which the applier
	// wraps into multi-row INSERTs. Tables are applied in foreign key order.
	FormatTuples Format = "tuples"

	// FormatRowLog is a single interleaved file of (table, rendered tuple) in walk order.
	//
	// It exists because grouping rows by table is exactly what row-insert mode cannot do.
	// That mode is the fallback for a schema whose foreign keys form a cycle, and a cycle
	// has no table order that keeps a parent ahead of its children — only the walk does,
	// so only the walk's own order can be replayed.
	FormatRowLog Format = "rowlog"

	// FormatValues is one file per table of typed row values, appended straight back into
	// the driver's own column builders at apply time. ClickHouse inserts columnar and
	// typed rather than as SQL text, so rendering to literals would change both the insert
	// path and how types are handled.
	FormatValues Format = "values"
)

// Codec owns what a segment looks like on disk.
type Codec interface {
	// Format is what gets recorded in the manifest.
	Format() Format

	// OpenSegment prepares the writers for a new segment in dir, which already exists.
	OpenSegment(dir string) (SegmentWriter, error)

	// Verify checks a sealed segment's files against its manifest. Only the manifest is
	// fsynced on seal, so a machine crash can leave a data file short and this is what
	// catches it before anything reaches the database.
	Verify(dir string, manifest *Manifest) error
}

// SegmentWriter accumulates one segment's rows.
type SegmentWriter interface {
	// WriteRow appends one row of the named table.
	WriteRow(table string, values []any) error

	// PendingBytes is how much has been written so far, from the writers' own counters
	// rather than a stat() per call. It is what the sizer compares against.
	PendingBytes() int64

	// Seal closes every stream and fills in the manifest's file records.
	Seal(manifest *Manifest) error

	// Discard closes and abandons a segment that will never be applied.
	Discard()
}

// Applier sends sealed segments to the database.
type Applier interface {
	// EnsureSchema creates whatever bookkeeping the applier needs, once, before any
	// segment is written.
	EnsureSchema(ctx context.Context) error

	// AlreadyApplied reports a segment recovery found on disk that the database already
	// holds, so it can be dropped rather than replayed.
	//
	// A driver with transactions answers this from its own record of applied segments. One
	// without answers it from how far the stored cursor got: replaying a segment it had in
	// fact applied duplicates exactly the rows re-streaming it would have duplicated, which
	// is the guarantee such a driver already ships with.
	AlreadyApplied(ctx context.Context, manifest *Manifest) (bool, error)

	// Apply loads one segment and advances the cursor with it.
	Apply(ctx context.Context, dir string, manifest *Manifest) error
}
