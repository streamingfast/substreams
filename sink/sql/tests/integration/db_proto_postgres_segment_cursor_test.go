package tests

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	"github.com/cenkalti/backoff/v4"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"github.com/streamingfast/bstream"
	"github.com/streamingfast/substreams/manifest"
	sink "github.com/streamingfast/substreams/sink"
	"github.com/streamingfast/substreams/sink/sql/db_proto"
	protosql "github.com/streamingfast/substreams/sink/sql/db_proto/sql"
	"github.com/streamingfast/substreams/sink/sql/db_proto/sql/spool"
	pbrelations "github.com/streamingfast/substreams/sink/sql/tests/relations"
	"github.com/stretchr/testify/require"
)

// TestDbProtoPostgresSegmentCarriesItsOwnCursor pins the one thing that makes a recorded
// segment trustworthy: the cursor committed with it has to cover the blocks it holds.
//
// A segment sealed before the cursor covering it was recorded carries the previous flush's
// instead, so it commits claiming a range its own cursor stops short of. The next run
// resumes at that cursor and Run's undo deletes every row above it — the segment's own —
// while the record stays behind to answer AlreadyApplied for the segment carrying exactly
// those rows. Recovery then discards that one and replays the segments behind it, moving
// the cursor over a gap it just created.
func TestDbProtoPostgresSegmentCarriesItsOwnCursor(t *testing.T) {
	outputMessageDescriptor := (*pbrelations.Output)(nil).ProtoReflect().Descriptor()
	postgresContainer := sharedDbChangesPostgresContainer
	ctx := context.Background()

	responses := []interface{}{
		relationsBlockData(t, "1a", "2025-01-01", entityCustomer("customer-1", "alpha")),
		relationsBlockData(t, "2a", "2025-01-02", entityCustomer("customer-2", "beta")),
		relationsBlockData(t, "3a", "2025-01-03", entityCustomer("customer-3", "gamma")),
		relationsBlockData(t, "4a", "2025-01-04", entityCustomer("customer-4", "delta")),
	}

	substreamsClientConfig := setupFakeSubstreamsServer(t, responses...)
	substreamsPackage := substreamsTestPackage(pbrelations.File_test_relations_relations_proto, outputMessageDescriptor)

	baseSink, err := sink.New(
		sink.SubstreamsModeProduction,
		false,
		substreamsPackage,
		substreamsPackage.Modules.Modules[0],
		manifest.ModuleHash{},
		substreamsClientConfig,
		logger,
		tracer,
		sink.WithBlockRange(bstream.MustParseRange("1-5", bstream.WithExclusiveEnd())),
		sink.WithRetryBackOff(&backoff.StopBackOff{}),
	)
	require.NoError(t, err)

	options := db_proto.SinkerFactoryOptions{
		UseProtoOption:  true,
		Constraints:     protosql.ConstraintPolicy{Timing: protosql.ConstraintsManual},
		UseTransactions: true,
		DecodeBatchSize: 1,
		// One byte of segment: every flush seals, so each block records a segment of its
		// own and the cursor each one carries is visible in the table.
		Spool: &spool.Options{Dir: t.TempDir(), SegmentMaxBytes: 1},
	}.Defaults()

	testSchema := "segment_carries_its_own_cursor"
	createPostgresTestSchema(t, postgresContainer.ConnectionString, testSchema)

	dbSinker, err := db_proto.SinkerFactory(baseSink, defaultOutputModuleName, outputMessageDescriptor, options)(ctx, postgresContainer.ConnectionString, testSchema, logger, tracer)
	require.NoError(t, err)

	require.NoError(t, dbSinker.Run(ctx))
	require.NoError(t, dbSinker.Err())

	db, err := sql.Open("postgres", postgresContainer.ConnectionString)
	require.NoError(t, err)
	dbx := sqlx.NewDb(db, "postgres").Unsafe()
	defer dbx.Close()

	var recorded []struct {
		FirstBlock uint64 `db:"first_block"`
		LastBlock  uint64 `db:"last_block"`
		Cursor     string `db:"cursor"`
	}
	require.NoError(t, dbx.Select(&recorded, fmt.Sprintf(
		`SELECT first_block, last_block, cursor FROM "%s"."_segments_" ORDER BY first_block`, testSchema)))
	require.NotEmpty(t, recorded, "the run has to have recorded at least one segment")

	for _, segment := range recorded {
		cursor, err := sink.NewCursor(segment.Cursor)
		require.NoError(t, err)

		require.GreaterOrEqualf(t, cursor.Block().Num(), segment.LastBlock,
			"the segment covering blocks %d-%d committed with a cursor at block %d, which does not cover it",
			segment.FirstBlock, segment.LastBlock, cursor.Block().Num())
	}
}

// TestDbProtoPostgresDropsSegmentRecordsPastTheCursor covers the records a database synced
// by an earlier build is still carrying, and anything else that could leave one behind.
//
// Run undoes every row above the stored cursor before a block arrives, so a record
// reaching past it describes rows that are no longer there. Left in place it answers
// AlreadyApplied for the segment carrying exactly those rows.
func TestDbProtoPostgresDropsSegmentRecordsPastTheCursor(t *testing.T) {
	outputMessageDescriptor := (*pbrelations.Output)(nil).ProtoReflect().Descriptor()
	postgresContainer := sharedDbChangesPostgresContainer
	ctx := context.Background()

	testSchema := "segment_records_past_the_cursor"
	createPostgresTestSchema(t, postgresContainer.ConnectionString, testSchema)

	openSink := func(t *testing.T) {
		t.Helper()

		substreamsClientConfig := setupFakeSubstreamsServer(t,
			relationsBlockData(t, "1a", "2025-01-01", entityCustomer("customer-1", "alpha")))
		substreamsPackage := substreamsTestPackage(pbrelations.File_test_relations_relations_proto, outputMessageDescriptor)

		baseSink, err := sink.New(
			sink.SubstreamsModeProduction,
			false,
			substreamsPackage,
			substreamsPackage.Modules.Modules[0],
			manifest.ModuleHash{},
			substreamsClientConfig,
			logger,
			tracer,
			sink.WithBlockRange(bstream.MustParseRange("1-2", bstream.WithExclusiveEnd())),
			sink.WithRetryBackOff(&backoff.StopBackOff{}),
		)
		require.NoError(t, err)

		options := db_proto.SinkerFactoryOptions{
			UseProtoOption:  true,
			Constraints:     protosql.ConstraintPolicy{Timing: protosql.ConstraintsManual},
			UseTransactions: true,
			DecodeBatchSize: 1,
			Spool:           &spool.Options{Dir: t.TempDir(), SegmentMaxBytes: 1},
		}.Defaults()

		dbSinker, err := db_proto.SinkerFactory(baseSink, defaultOutputModuleName, outputMessageDescriptor, options)(ctx, postgresContainer.ConnectionString, testSchema, logger, tracer)
		require.NoError(t, err)
		require.NoError(t, dbSinker.Run(ctx))
		require.NoError(t, dbSinker.Err())
	}

	openSink(t)

	db, err := sql.Open("postgres", postgresContainer.ConnectionString)
	require.NoError(t, err)
	dbx := sqlx.NewDb(db, "postgres").Unsafe()
	defer dbx.Close()

	// A record reaching well past anything the run reached, as an interrupted seal under
	// the old ordering would have left.
	_, err = dbx.Exec(fmt.Sprintf(
		`INSERT INTO "%s"."_segments_" (first_block, last_block, cursor) VALUES (900, 999, 'stale')`, testSchema))
	require.NoError(t, err)

	// Opening the sink again is what runs the pass.
	openSink(t)

	var remaining int
	require.NoError(t, dbx.Get(&remaining, fmt.Sprintf(
		`SELECT count(*) FROM "%s"."_segments_" WHERE first_block = 900`, testSchema)))
	require.Zero(t, remaining, "the record the stored cursor does not cover must be gone")
}
