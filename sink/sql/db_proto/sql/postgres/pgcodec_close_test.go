package postgres

import (
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	sqlbytes "github.com/streamingfast/substreams/sink/sql/bytes"
	"github.com/streamingfast/substreams/sink/sql/db_proto/sql/postgres/pgcopy"
	"github.com/streamingfast/substreams/sink/sql/db_proto/sql/spool"
	"github.com/stretchr/testify/require"
)

// TestSealClosesEveryStreamFile covers the descriptors a sealed segment must not keep.
//
// A segment is sealed and forgotten, so whatever it opened has to be released by Seal. A
// leak here is invisible until a long backfill runs out of descriptors, and the applier's
// os.RemoveAll frees no space while they are held.
func TestSealClosesEveryStreamFile(t *testing.T) {
	const tableName = "payloads"

	for _, format := range []spool.Format{spool.FormatPGCopy, spool.FormatTuples} {
		t.Run(string(format), func(t *testing.T) {
			codec := newPGCodec(
				format,
				map[string]*pgcopy.Table{
					tableName: {
						Schema: "public",
						Name:   tableName,
						Columns: []pgcopy.Column{
							{Name: "payload", OID: pgtype.TextOID},
						},
					},
				},
				sqlbytes.EncodingHex,
			)
			segment := &pgSegment{
				dir:    t.TempDir(),
				codec:  codec,
				tables: map[string]*pgTableFile{},
			}

			require.NoError(t, segment.WriteRow(tableName, []any{[]byte{0xde, 0xad}}))
			require.NoError(t, segment.Seal(&spool.Manifest{}))

			// Closing an already closed file is what says the seal released it.
			require.Error(t, segment.tables[tableName].file.Close())
		})
	}
}
