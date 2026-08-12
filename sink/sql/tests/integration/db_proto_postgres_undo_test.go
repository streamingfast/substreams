package tests

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"

	"github.com/cenkalti/backoff/v4"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"github.com/streamingfast/bstream"
	"github.com/streamingfast/substreams/manifest"
	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
	sink "github.com/streamingfast/substreams/sink"
	"github.com/streamingfast/substreams/sink/sql/db_proto"
	"github.com/streamingfast/substreams/sink/sql/db_proto/sql/spool"
	pbrelations "github.com/streamingfast/substreams/sink/sql/tests/relations"
	"github.com/stretchr/testify/require"
)

// TestDbProtoPostgresUndo covers a reorg in every shape the from-proto sink runs in.
//
// The undo used to be a single DELETE on _blocks_, leaning on `fk_block ... ON DELETE
// CASCADE` to take the entity rows with it. That foreign key only exists with
// constraints, so without them a reorg deleted the block rows and left every
// entity row of those blocks behind — and running without constraints is now the default.
func TestDbProtoPostgresUndo(t *testing.T) {
	outputMessageDescriptor := (*pbrelations.Output)(nil).ProtoReflect().Descriptor()
	postgresContainer := sharedDbChangesPostgresContainer

	tests := []struct {
		name           string
		useConstraints bool
		localBuffer    bool
	}{
		{"without constraints", false, false},
		{"with constraints", true, false},
		{"with the local buffer", false, true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			responses := []*pbsubstreamsrpc.Response{
				relationsBlockData(t, "1a", "2025-01-01", entityCustomer("customer-1", "kept")),
				relationsBlockData(t, "2a", "2025-01-02", entityCustomer("customer-2", "undone")),
				relationsBlockData(t, "3a", "2025-01-03", entityCustomer("customer-3", "undone")),
				// The fork takes blocks 2 and 3 back.
				blockUndo(t, "1a"),
			}

			pattern := make([]interface{}, len(responses))
			for i, response := range responses {
				pattern[i] = response
			}
			substreamsClientConfig := setupFakeSubstreamsServer(t, pattern...)
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
			)
			require.NoError(t, err)

			options := db_proto.SinkerFactoryOptions{
				UseProtoOption:  true,
				Constraints:     constraintPolicy(test.useConstraints),
				UseTransactions: true,
				// One block per batch, so the undone blocks are in the database rather
				// than still held in memory when the signal arrives.
				DecodeBatchSize: 1,
			}.Defaults()

			if test.localBuffer {
				options.Spool = &spool.Options{Dir: t.TempDir()}
			}

			testSchema := "undo_" + strings.ReplaceAll(strings.ToLower(test.name), " ", "_")
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

			count := func(table string) int {
				var out int
				require.NoError(t, dbx.Get(&out, fmt.Sprintf(`SELECT count(*) FROM "%s"."%s"`, testSchema, table)))

				return out
			}

			require.Equal(t, 1, count("_blocks_"), "only the last valid block survives the undo")
			require.Equal(t, 1, count("customers"), "the entity rows of the undone blocks must be gone, not orphaned")
		})
	}
}
