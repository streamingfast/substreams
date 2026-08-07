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
	pbrelations "github.com/streamingfast/substreams/sink/sql/tests/relations"
	"github.com/stretchr/testify/require"
)

// TestDbProtoPostgresBoundedRunFlushesOnCompletion locks in the completion flush for the
// from-proto sinker.
//
// Blocks accumulate until --block-batch-size worth of them have gone by. A bounded run
// that ends mid-batch used to leave those blocks in memory: nothing drained them when
// the stream reached its stop block, so the rows and the cursor covering them were
// silently dropped and the run still reported success.
//
// The batch size here is larger than the range, so the periodic flush can never fire
// and the completion flush is the only writer. Against the previous code this test finds
// an empty database.
func TestDbProtoPostgresBoundedRunFlushesOnCompletion(t *testing.T) {
	const (
		schema         = "bounded_run_completion"
		blocks         = 5
		blockBatchSize = 1000 // far beyond the range: only the completion flush can write
	)

	outputMessageDescriptor := (*pbrelations.Output)(nil).ProtoReflect().Descriptor()
	postgresContainer := sharedDbChangesPostgresContainer

	responses := make([]any, 0, blocks)
	for block := 1; block <= blocks; block++ {
		responses = append(responses, relationsBlockData(t,
			fmt.Sprintf("%da", block), "2025-01-01",
			entityCustomer(fmt.Sprintf("cust-%d", block), fmt.Sprintf("Customer %d", block)),
		))
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
		sink.WithBlockRange(bstream.MustParseRange(fmt.Sprintf("1-%d", blocks+1), bstream.WithExclusiveEnd())),
		sink.WithRetryBackOff(&backoff.StopBackOff{}),
	)
	require.NoError(t, err)

	options := db_proto.SinkerFactoryOptions{
		UseProtoOption:  true,
		UseConstraints:  false,
		UseTransactions: true,
		BlockBatchSize:  blockBatchSize,
	}.Defaults()
	options.BlockBatchSize = blockBatchSize

	createPostgresTestSchema(t, postgresContainer.ConnectionString, schema)

	ctx := context.Background()
	dbSinker, err := db_proto.SinkerFactory(baseSink, defaultOutputModuleName, outputMessageDescriptor, options)(
		ctx, postgresContainer.ConnectionString, schema, logger, tracer)
	require.NoError(t, err)

	require.NoError(t, dbSinker.Run(ctx))
	require.NoError(t, dbSinker.Err())

	db, err := sql.Open("postgres", postgresContainer.ConnectionString)
	require.NoError(t, err)
	dbx := sqlx.NewDb(db, "postgres").Unsafe()
	defer dbx.Close()

	var customerCount int
	require.NoError(t, dbx.QueryRow(fmt.Sprintf(`SELECT count(*) FROM %q.customers`, schema)).Scan(&customerCount))
	require.Equal(t, blocks, customerCount,
		"the blocks held when the range completed were never written")

	var blockCount int
	require.NoError(t, dbx.QueryRow(fmt.Sprintf(`SELECT count(*) FROM %q._blocks_`, schema)).Scan(&blockCount))
	require.Equal(t, blocks, blockCount)

	// The cursor must cover the flushed blocks, otherwise a restart would re-stream and
	// duplicate them.
	var cursor string
	require.NoError(t, dbx.QueryRow(fmt.Sprintf(`SELECT cursor FROM %q._cursor_ WHERE name = 'cursor'`, schema)).Scan(&cursor))
	require.NotEmpty(t, cursor, "no cursor was stored for the completed range")
}
