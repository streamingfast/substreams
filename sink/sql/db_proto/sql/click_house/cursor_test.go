package clickhouse

import (
	"path/filepath"
	"testing"

	"github.com/streamingfast/bstream"
	sink "github.com/streamingfast/substreams/sink"
	"github.com/stretchr/testify/require"
)

// TestStoreCursorFileWritesWhereFetchCursorReads pins the path the spool applier uses.
//
// StoreCursor records onto the open segment whenever a spool is active, so this is the
// only thing that makes a spooled ClickHouse backfill resumable: routing the applier
// through StoreCursor instead leaves the file untouched for the whole run.
func TestStoreCursorFileWritesWhereFetchCursorReads(t *testing.T) {
	database := &Database{cursorFilePath: filepath.Join(t.TempDir(), "cursor.txt")}

	block := bstream.NewBlockRef("10a", 10)
	opaque := (&bstream.Cursor{
		Step:      bstream.StepNewIrreversible,
		Block:     block,
		HeadBlock: block,
		LIB:       block,
	}).ToOpaque()

	cursor, err := sink.NewCursor(opaque)
	require.NoError(t, err)
	require.NoError(t, database.storeCursorFile(cursor))

	read, err := database.FetchCursor()
	require.NoError(t, err)
	require.Equal(t, uint64(10), read.Block().Num())
}
