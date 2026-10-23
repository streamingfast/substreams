package sinker

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	pbdatabase "github.com/streamingfast/substreams-sink-database-changes/pb/sf/substreams/sink/database/v1"
	"github.com/streamingfast/substreams/manifest"
	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	sink "github.com/streamingfast/substreams/sink"
	db2 "github.com/streamingfast/substreams/sink/sql/db_changes/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestUndoAfterRestart_Postgres stops a sink holding reversible blocks, then has another sink resume
// from the stored cursor and receive an undo of one of them: the undo history must still be there.
func TestUndoAfterRestart_Postgres(t *testing.T) {
	ctx := context.Background()
	dsnString := os.Getenv("PG_DSN")
	if dsnString == "" {
		t.Skip(`PG_DSN not set, please specify PG_DSN to run this test, example: PG_DSN="psql://dev-node:insecure-change-me-in-prod@localhost:5432/dev-node?sslmode=disable"`)
	}

	dsn, err := db2.ParseDSN(dsnString)
	require.NoError(t, err)
	raw, err := sql.Open("postgres", dsn.ConnString())
	require.NoError(t, err)
	defer raw.Close()

	schema := fmt.Sprintf("takeover_test_%d", time.Now().UnixNano())
	_, err = raw.Exec(fmt.Sprintf(`CREATE SCHEMA %s; CREATE TABLE %s.xfer (id text PRIMARY KEY, "from" text, "to" text)`, schema, schema))
	require.NoError(t, err)
	defer raw.Exec(fmt.Sprintf("DROP SCHEMA %s CASCADE", schema))
	scoped := dsnString + "&schemaName=" + schema

	newSinker := func() (*SQLSinker, *db2.Loader) {
		dsn, err := db2.ParseDSN(scoped)
		require.NoError(t, err)
		// No flush before the stream ends: the blocks are only written by the final flush
		l, err := db2.NewLoader(dsn, "cursors", "substreams_history", "", 1000, 1_000_000, 1000, "error", nil, logger, tracer)
		require.NoError(t, err)
		require.NoError(t, l.LoadTables(schema, "cursors", "substreams_history"))
		s, err := sink.NewFromConfig(&sink.SinkerConfig{
			Pkg:              testPackage,
			OutputModule:     testPackage.Modules.Modules[0],
			OutputModuleHash: manifest.ModuleHash([]byte("unused")),
			ClientConfig:     testClientConfig,
			Mode:             sink.SubstreamsModeDevelopment,
			Logger:           logger,
		})
		require.NoError(t, err)
		sinker, err := New(s, l, logger, nil, 3, time.Second)
		require.NoError(t, err)
		return sinker, l
	}

	setup, err := db2.ParseDSN(scoped)
	require.NoError(t, err)
	setupLoader, err := db2.NewLoader(setup, "cursors", "substreams_history", "", 0, 0, 0, "error", nil, logger, tracer)
	require.NoError(t, err)
	require.NoError(t, setupLoader.Setup(ctx, schema, "", false))
	setupLoader.Close()

	// The active sink handles two reversible blocks (final block 5), then is stopped
	a, la := newSinker()
	// Loading the cursor as Run does
	_, _, _, err = la.GetCursor(ctx, a.OutputModuleHash())
	require.ErrorIs(t, err, db2.ErrCursorNotFound)
	require.NoError(t, la.InsertCursor(ctx, a.OutputModuleHash(), sink.NewBlankCursor()))
	la.SetExpectedCursor(sink.NewBlankCursor().String())

	live := true
	for _, n := range []uint64{10, 11} {
		require.NoError(t, a.HandleBlockScopedData(ctx,
			blockScopedData("db_out", []*pbdatabase.TableChange{insertRowSinglePK("xfer", fmt.Sprintf("%d", n), "from", "a", "to", "b")}, n, 5),
			&live, sink.MustNewCursor(simpleCursor(n, 5))))
	}
	a.Shutdown(nil)
	require.NoError(t, a.HandleBlockRangeCompletion(ctx, sink.MustNewCursor(simpleCursor(11, 5))))
	require.NoError(t, la.Close())

	// A standby takes over from the stored cursor and the chain undoes block 11
	b, lb := newSinker()
	cursor, rawCursor, _, err := lb.GetCursor(ctx, b.OutputModuleHash())
	require.NoError(t, err)
	lb.SetExpectedCursor(rawCursor)
	require.Equal(t, uint64(11), cursor.Block().Num())

	require.NoError(t, b.HandleBlockUndoSignal(ctx, &pbsubstreamsrpc.BlockUndoSignal{
		LastValidBlock:  &pbsubstreams.BlockRef{Id: "10", Number: 10},
		LastValidCursor: simpleCursor(10, 5),
	}, sink.MustNewCursor(simpleCursor(10, 5))))

	var ids []string
	rows, err := raw.Query(fmt.Sprintf(`SELECT id FROM %s.xfer ORDER BY id`, schema))
	require.NoError(t, err)
	for rows.Next() {
		var id string
		require.NoError(t, rows.Scan(&id))
		ids = append(ids, id)
	}
	require.NoError(t, rows.Err())
	assert.Equal(t, []string{"10"}, ids, "the row of the undone block must be reverted")

	var blockNum uint64
	require.NoError(t, raw.QueryRow(fmt.Sprintf(`SELECT block_num FROM %s.cursors`, schema)).Scan(&blockNum))
	assert.Equal(t, uint64(10), blockNum)
	require.NoError(t, lb.Close())
}
