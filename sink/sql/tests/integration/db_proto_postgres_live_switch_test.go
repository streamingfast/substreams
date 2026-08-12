package tests

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/cenkalti/backoff/v4"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"github.com/streamingfast/bstream"
	"github.com/streamingfast/substreams/manifest"
	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
	sink "github.com/streamingfast/substreams/sink"
	"github.com/streamingfast/substreams/sink/sql/db_proto"
	protosql "github.com/streamingfast/substreams/sink/sql/db_proto/sql"
	"github.com/streamingfast/substreams/sink/sql/db_proto/sql/spool"
	pbrelations "github.com/streamingfast/substreams/sink/sql/tests/relations"
	"github.com/stretchr/testify/require"
)

// TestDbProtoPostgresLiveSwitch pins what happens when a buffered backfill reaches the
// chain head.
//
// The local buffer holds rows on disk until a segment fills, which is what a backfill
// wants and the opposite of what a live sink wants: at the head a block should be
// queryable when it arrives. Reaching a live block therefore drains the buffer and
// switches the database to direct inserts, once and for good.
func TestDbProtoPostgresLiveSwitch(t *testing.T) {
	outputMessageDescriptor := (*pbrelations.Output)(nil).ProtoReflect().Descriptor()
	postgresContainer := sharedDbChangesPostgresContainer

	// Blocks 1 and 2 are irreversible, so they are backfill and stay in the buffer; block
	// 3 arrives undo-able, which is what the cursor-based checker calls live.
	irreversible := func(blockIdentifier, blockTime string, entities ...*pbrelations.Entity) *pbsubstreamsrpc.Response {
		return blockScopedData(t, blockIdentifier, &pbrelations.Output{Entities: entities}, blockTimepb(t, blockTime), finalBlock(blockIdentifier))
	}

	responses := []interface{}{
		irreversible("1a", "2025-01-01", entityCustomer("customer-1", "backfilled")),
		irreversible("2a", "2025-01-02", entityCustomer("customer-2", "backfilled")),
		relationsBlockData(t, "3a", "2025-01-03", entityCustomer("customer-3", "live")),
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
		sink.WithBlockRange(bstream.MustParseRange("1-4", bstream.WithExclusiveEnd())),
		sink.WithRetryBackOff(&backoff.StopBackOff{}),
		sink.WithLivenessChecker(sink.NewCursorBasedLivenessChecker()),
	)
	require.NoError(t, err)

	bufferDir := t.TempDir()
	options := db_proto.SinkerFactoryOptions{
		UseProtoOption:  true,
		Constraints:     protosql.DisableAllConstraints(),
		UseTransactions: true,
		// Large enough that nothing would be flushed on batch size alone, so what lands
		// in the database is what the switch and the live path put there.
		DecodeBatchSize: 100,
		Spool:           &spool.Options{Dir: bufferDir},
	}.Defaults()

	testSchema := "live_switch"
	createPostgresTestSchema(t, postgresContainer.ConnectionString, testSchema)

	ctx := context.Background()
	dbSinker, err := db_proto.SinkerFactory(baseSink, defaultOutputModuleName, outputMessageDescriptor, options)(ctx, postgresContainer.ConnectionString, testSchema, logger, tracer)
	require.NoError(t, err)

	require.NoError(t, dbSinker.Run(ctx))
	require.NoError(t, dbSinker.Err())

	db, err := sql.Open("postgres", postgresContainer.ConnectionString)
	require.NoError(t, err)
	dbx := sqlx.NewDb(db, "postgres").Unsafe()
	defer dbx.Close()

	var customers int
	require.NoError(t, dbx.Get(&customers, fmt.Sprintf(`SELECT count(*) FROM "%s"."customers"`, testSchema)))
	require.Equal(t, 3, customers, "every block reaches the database, whether it went through the buffer or straight in")

	// The buffer keeps one directory per schema and removes each segment as it is
	// applied, so a drained and closed buffer leaves nothing behind.
	segments, err := filepath.Glob(filepath.Join(bufferDir, testSchema, "seg-*"))
	require.NoError(t, err)
	require.Empty(t, segments, "the buffer must be drained by the switch, not left holding segments")
}

// TestDbProtoPostgresSwitchToDirectInserts is the switch on its own, where buffering is
// observable: a full run drains at the end whatever happened in the middle, so only a
// mid-run look at the database can tell a buffered write from a direct one.
func TestDbProtoPostgresSwitchToDirectInserts(t *testing.T) {
	outputMessageDescriptor := (*pbrelations.Output)(nil).ProtoReflect().Descriptor()
	postgresContainer := sharedDbChangesPostgresContainer
	ctx := context.Background()

	testSchema := "switch_to_direct"
	createPostgresTestSchema(t, postgresContainer.ConnectionString, testSchema)

	bufferDir := t.TempDir()
	options := db_proto.SinkerFactoryOptions{
		UseProtoOption:  true,
		Constraints:     protosql.DisableAllConstraints(),
		UseTransactions: true,
		DecodeBatchSize: 1,
		Spool:           &spool.Options{Dir: bufferDir},
	}.Defaults()

	database, err := db_proto.SetupDatabaseSchema(ctx, postgresContainer.ConnectionString, testSchema, defaultOutputModuleName, outputMessageDescriptor, options, logger, tracer)
	require.NoError(t, err)
	require.NoError(t, database.Open())
	defer database.Close(ctx)

	db, err := sql.Open("postgres", postgresContainer.ConnectionString)
	require.NoError(t, err)
	dbx := sqlx.NewDb(db, "postgres").Unsafe()
	defer dbx.Close()

	countCustomers := func() int {
		var out int
		require.NoError(t, dbx.Get(&out, fmt.Sprintf(`SELECT count(*) FROM "%s"."customers"`, testSchema)))

		return out
	}

	write := func(blockNum uint64, customerID string) {
		t.Helper()

		require.NoError(t, database.BeginTransaction())
		require.NoError(t, database.InsertBlock(blockNum, fmt.Sprintf("%da", blockNum), fixedBaseTime))
		require.NoError(t, database.Insert("customers", []any{blockNum, fixedBaseTime, customerID, "name"}))
		cursor := bstream.Cursor{
			Step:      bstream.StepNewIrreversible,
			Block:     bstream.NewBlockRef(fmt.Sprintf("%da", blockNum), blockNum),
			HeadBlock: bstream.NewBlockRef(fmt.Sprintf("%da", blockNum), blockNum),
			LIB:       bstream.NewBlockRef(fmt.Sprintf("%da", blockNum), blockNum),
		}
		sinkCursor, err := sink.NewCursor(cursor.ToOpaque())
		require.NoError(t, err)
		require.NoError(t, database.StoreCursor(sinkCursor))
		_, err = database.Flush()
		require.NoError(t, err)
		require.NoError(t, database.CommitTransaction())
	}

	write(1, "customer-1")
	require.Zero(t, countCustomers(), "a buffered write must not have reached the database yet")

	require.NoError(t, database.SwitchToDirectInserts(ctx))
	require.Equal(t, 1, countCustomers(), "the switch drains what the buffer was holding")

	write(2, "customer-2")
	require.Equal(t, 2, countCustomers(), "after the switch a write is visible as soon as it commits")

	segments, err := filepath.Glob(filepath.Join(bufferDir, testSchema, "seg-*"))
	require.NoError(t, err)
	require.Empty(t, segments, "the buffer is closed for good, nothing accumulates in it")
}
