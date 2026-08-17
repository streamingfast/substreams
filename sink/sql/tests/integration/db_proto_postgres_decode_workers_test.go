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
	pbrelations "github.com/streamingfast/substreams/sink/sql/tests/relations"
	"github.com/stretchr/testify/require"
)

// TestDbProtoPostgresDecodeWorkers asserts that decoding blocks concurrently produces
// exactly the same database as decoding them one at a time.
//
// The workers unmarshal and walk in parallel but only fill a per-block buffer; the
// inserts are replayed in block order on the flush goroutine. That ordering is what
// keeps a parent row ahead of the children that reference it, so a divergence between
// one worker and many is the failure this test exists to catch.
func TestDbProtoPostgresDecodeWorkers(t *testing.T) {
	// blockBatchSize 3 over blocks 1..8 flushes twice, four blocks at a time, so the
	// pool actually has work to spread.
	const blockBatchSize = 3

	serial := runDecodeWorkersSinker(t, "decode_workers_serial", 1, blockBatchSize)
	defer serial.Close()

	parallel := runDecodeWorkersSinker(t, "decode_workers_parallel", 8, blockBatchSize)
	defer parallel.Close()

	for _, table := range []string{"customers", "orders", "order_items", "order_extensions", "_blocks_"} {
		serialRows := dumpTable(t, serial, "decode_workers_serial", table)
		parallelRows := dumpTable(t, parallel, "decode_workers_parallel", table)

		require.NotEmpty(t, serialRows, "table %q came out empty, the test would prove nothing", table)
		require.Equal(t, serialRows, parallelRows,
			"table %q differs between 1 and 8 decode workers", table)
	}

	// The comparison above is differential, so it cannot see an ordering mistake that
	// both runs share. Assert the absolute order too: blocks must reach the database
	// ascending, whatever order the workers happened to finish in.
	for _, schema := range []string{"decode_workers_serial", "decode_workers_parallel"} {
		dbx := serial
		if schema == "decode_workers_parallel" {
			dbx = parallel
		}

		blocks := dumpTable(t, dbx, schema, "_blocks_")
		require.Len(t, blocks, 8)
		for i, line := range blocks {
			require.Contains(t, line, fmt.Sprintf("number=%d|", i+1),
				"%s._blocks_ row %d is out of order: %s", schema, i, line)
		}
	}
}

// dumpTable renders a whole table as text, in physical row order, so two databases can
// be compared without knowing the column types.
//
// Ordering by ctid is what makes this test able to see the replay order at all: the row
// *values* here do not depend on insert order, so comparing the tables as sets would
// pass even if the blocks were applied backwards. These tables are only ever appended
// to within a transaction, so ctid order is insert order.
func dumpTable(t *testing.T, dbx *sqlx.DB, schema, table string) []string {
	t.Helper()

	rows, err := dbx.Query(fmt.Sprintf(`SELECT * FROM %q.%q ORDER BY ctid`, schema, table))
	require.NoError(t, err)
	defer rows.Close()

	columns, err := rows.Columns()
	require.NoError(t, err)

	var out []string
	for rows.Next() {
		values := make([]any, len(columns))
		pointers := make([]any, len(columns))
		for i := range values {
			pointers[i] = &values[i]
		}
		require.NoError(t, rows.Scan(pointers...))

		line := ""
		for i, v := range values {
			line += fmt.Sprintf("%s=%v|", columns[i], v)
		}
		out = append(out, line)
	}
	require.NoError(t, rows.Err())

	return out
}

func runDecodeWorkersSinker(t *testing.T, schema string, decodeWorkers, blockBatchSize int) *sqlx.DB {
	t.Helper()

	outputMessageDescriptor := (*pbrelations.Output)(nil).ProtoReflect().Descriptor()
	postgresContainer := sharedDbChangesPostgresContainer

	responses := make([]any, 0, 8)
	for block := 1; block <= 8; block++ {
		responses = append(responses, relationsBlockData(t,
			fmt.Sprintf("%da", block), "2025-01-01",
			entityCustomer(fmt.Sprintf("cust-%d", block), fmt.Sprintf("Customer %d", block)),
			entityOrder(
				fmt.Sprintf("order-%d", block),
				fmt.Sprintf("cust-%d", block),
				orderExtension(fmt.Sprintf("extension for order %d", block)),
				orderItem(fmt.Sprintf("item-%d-a", block), int64(block)),
				orderItem(fmt.Sprintf("item-%d-b", block), int64(block*2)),
			),
			entityItem(fmt.Sprintf("item-%d", block), fmt.Sprintf("Item %d", block), float64(block)*1.5),
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
		sink.WithBlockRange(bstream.MustParseRange("1-9", bstream.WithExclusiveEnd())),
		sink.WithRetryBackOff(&backoff.StopBackOff{}),
	)
	require.NoError(t, err)

	options := db_proto.SinkerFactoryOptions{
		UseProtoOption:  true,
		Constraints:     protosql.DisableAllConstraints(),
		UseTransactions: true,
		DecodeBatchSize: blockBatchSize,
		DecodeWorkers:   decodeWorkers,
	}.Defaults()
	options.DecodeWorkers = decodeWorkers
	options.DecodeBatchSize = blockBatchSize

	createPostgresTestSchema(t, postgresContainer.ConnectionString, schema)

	ctx := context.Background()
	dbSinker, err := db_proto.SinkerFactory(baseSink, defaultOutputModuleName, outputMessageDescriptor, options)(
		ctx, postgresContainer.ConnectionString, schema, logger, tracer)
	require.NoError(t, err)

	require.NoError(t, dbSinker.Run(ctx))
	require.NoError(t, dbSinker.Err())

	db, err := sql.Open("postgres", postgresContainer.ConnectionString)
	require.NoError(t, err)

	return sqlx.NewDb(db, "postgres").Unsafe()
}
