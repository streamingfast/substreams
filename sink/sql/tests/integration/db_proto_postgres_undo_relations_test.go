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
	sink "github.com/streamingfast/substreams/sink"
	"github.com/streamingfast/substreams/sink/sql/db_proto"
	"github.com/streamingfast/substreams/sink/sql/db_proto/sql/postgres/buffer"
	pbrelations "github.com/streamingfast/substreams/sink/sql/tests/relations"
	"github.com/stretchr/testify/require"
)

// TestDbProtoPostgresUndoRelations undoes a reorg over the full relational shape rather
// than a single flat table: nesting (order_extensions and order_items are children of
// orders) and sibling references (orders points at customers, order_items at items).
//
// Those two need opposite things from a delete order. Children have to go before their
// parent, and a referencing table before the table it points at — which is the same rule
// stated twice only if the reference graph is a tree. It is not, so the deletes follow
// the reverse of the schema's topological order.
func TestDbProtoPostgresUndoRelations(t *testing.T) {
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
			blockEntities := func(suffix string) []*pbrelations.Entity {
				return []*pbrelations.Entity{
					entityCustomer("customer-"+suffix, "name-"+suffix),
					entityItem("item-"+suffix, "item-name-"+suffix, 1.5),
					entityOrder("order-"+suffix, "customer-"+suffix,
						&pbrelations.OrderExtension{Description: "extension-" + suffix},
						&pbrelations.OrderItem{ItemId: "item-" + suffix, Quantity: 2},
					),
				}
			}

			responses := []interface{}{
				relationsBlockData(t, "1a", "2025-01-01", blockEntities("1")...),
				relationsBlockData(t, "2a", "2025-01-02", blockEntities("2")...),
				relationsBlockData(t, "3a", "2025-01-03", blockEntities("3")...),
				blockUndo(t, "1a"),
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
			)
			require.NoError(t, err)

			options := db_proto.SinkerFactoryOptions{
				UseProtoOption:  true,
				UseConstraints:  test.useConstraints,
				UseTransactions: true,
				BlockBatchSize:  1,
			}.Defaults()

			if test.localBuffer {
				options.LocalBuffer = &buffer.Options{Dir: t.TempDir()}
			}

			testSchema := "undo_relations_" + strings.ReplaceAll(test.name, " ", "_")
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

			// Only block 1 survives the undo, and it contributed exactly one row to each.
			for _, table := range []string{"_blocks_", "customers", "items", "orders", "order_items", "order_extensions"} {
				require.Equal(t, 1, count(table), "table %q keeps only what block 1 wrote", table)
			}
		})
	}
}
