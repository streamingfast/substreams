package tests

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "github.com/ClickHouse/clickhouse-go/v2"
	"github.com/cenkalti/backoff/v4"
	"github.com/jmoiron/sqlx"
	"github.com/streamingfast/bstream"
	"github.com/streamingfast/substreams/manifest"
	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
	sink "github.com/streamingfast/substreams/sink"
	"github.com/streamingfast/substreams/sink/sql/db_proto"
	protosql "github.com/streamingfast/substreams/sink/sql/db_proto/sql"
	pbrelations "github.com/streamingfast/substreams/sink/sql/tests/relations"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go/modules/clickhouse"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestDbProtoClickhouseIntegration(t *testing.T) {
	outputMessageDescriptor := (*pbrelations.Output)(nil).ProtoReflect().Descriptor()

	var dbDatabase string
	clickhouseDSN, _ := setupClickhouseContainer(t, func(ctx context.Context, user, password, database, dsn string, container *clickhouse.ClickHouseContainer) error {
		dbDatabase = database
		return nil
	})

	streamMock := func(responses ...*pbsubstreamsrpc.Response) []*pbsubstreamsrpc.Response {
		return responses
	}

	equalsOrderRows := func(expected []*OrderRow) func(t *testing.T, dbx *sqlx.DB) {
		return func(t *testing.T, dbx *sqlx.DB) {
			require.Equal(t, expected, readRowsBy[OrderRow](t, dbx, "orders", "order_id"))
		}
	}

	equalsTypesTestRows := func(expected []*TypesTestRow) func(t *testing.T, dbx *sqlx.DB) {
		return func(t *testing.T, dbx *sqlx.DB) {
			require.Equal(t, expected, readRowsBy[TypesTestRow](t, dbx, "types_tests", "id"))
		}
	}

	testCases := []struct {
		name      string
		responses []*pbsubstreamsrpc.Response
		expected  func(t *testing.T, dbx *sqlx.DB)
	}{
		{
			"single order",
			streamMock(
				relationsBlockData(t, "1a", "2025-01-01",
					entityOrder("o1", "c1", orderExtension("o1 desc"), orderItem("i1", 2), orderItem("i2", 3)),
				),
			),
			equalsOrderRows([]*OrderRow{
				{rowMeta(t, 1, "2025-01-01"), "o1", "c1"},
			}),
		},
		{
			"two orders",
			streamMock(
				relationsBlockData(t, "1a", "2025-01-01",
					entityOrder("o1", "c1", orderExtension("o1 desc"), orderItem("i1", 2), orderItem("i2", 3)),
					entityOrder("o2", "c2", orderExtension("o2 desc"), orderItem("i3", 1), orderItem("i4", 4)),
				),
			),
			equalsOrderRows([]*OrderRow{
				{rowMeta(t, 1, "2025-01-01"), "o1", "c1"},
				{rowMeta(t, 1, "2025-01-01"), "o2", "c2"},
			}),
		},
		{
			"order with optional uint256 empty string",
			streamMock(
				relationsBlockData(t, "1a", "2025-01-01",
					entityOrderWithOptionalUint256("o1", "c1", orderExtensionWithOptionalUint256("o1 desc"), orderItem("i1", 2)),
				),
			),
			equalsOrderRows([]*OrderRow{
				{rowMeta(t, 1, "2025-01-01"), "o1", "c1"},
			}),
		},
		{
			// This test case verifies that ClickHouse correctly handles empty repeated fields.
			// Unlike PostgreSQL, ClickHouse can infer the array type from the column definition
			// in the schema, so empty arrays work correctly.
			//
			// This test serves as a regression test to ensure ClickHouse continues to handle
			// empty arrays properly.
			"types_test with empty repeated string field",
			streamMock(
				relationsBlockData(t, "1a", "2025-01-01",
					entityTypesTestWithEmptyRepeatedString(1),
				),
			),
			equalsTypesTestRows([]*TypesTestRow{
				{rowMeta(t, 1, "2025-01-01"), 1},
			}),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Technically those tests are meant to be run in parallel, but for an unknown reason,
			// activating t.Parallel() leads to queries getting "rows" from different tests and I'm not sure why.
			// Database shows weirdly at the end correct results, need to be investigated.

			pattern := make([]interface{}, len(tc.responses))
			for i, resp := range tc.responses {
				pattern[i] = resp
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
				sink.WithBlockRange(bstream.MustParseRange("1-2", bstream.WithExclusiveEnd())),
				sink.WithRetryBackOff(&backoff.StopBackOff{}),
			)
			require.NoError(t, err)

			// Create sinker factory options
			clickhouseStateFolder := t.TempDir()

			options := db_proto.SinkerFactoryOptions{
				UseProtoOption:  true,
				Constraints:     protosql.DisableAllConstraints(),
				UseTransactions: true,
				DecodeBatchSize: 1,
				Clickhouse: db_proto.SinkerFactoryClickhouse{
					SinkInfoFolder: clickhouseStateFolder,
					CursorFilePath: filepath.Join(clickhouseStateFolder, "cursor.txt"),
				},
			}.Defaults()

			sinkerFactory := db_proto.SinkerFactory(
				baseSink,
				defaultOutputModuleName,
				outputMessageDescriptor,
				options,
			)

			testSchema := strings.ReplaceAll(strings.ToLower(tc.name), " ", "_")
			createTestDatabase(t, clickhouseDSN, testSchema)

			testDSN := strings.Replace(clickhouseDSN, dbDatabase, testSchema, 1)

			ctx := context.Background()
			dbSinker, err := sinkerFactory(ctx, testDSN, testSchema, logger, tracer)
			require.NoError(t, err)

			err = dbSinker.Run(ctx)
			require.NoError(t, err)
			require.NoError(t, dbSinker.Err())

			db, err := sql.Open("clickhouse", testDSN)
			require.NoError(t, err)

			// Ignore all column missing destination errors by using sqlx.Unsafe, as
			// we don't want to compare _version_ column for now.
			dbx := sqlx.NewDb(db, "clickhouse").Unsafe()
			defer dbx.Close()

			if tc.expected != nil {
				tc.expected(t, dbx)
			}
		})
	}
}

func createTestDatabase(t *testing.T, dsn string, schemaName string) {
	t.Helper()

	db, err := sql.Open("clickhouse", dsn)
	require.NoError(t, err)
	defer db.Close()

	_, err = db.Exec(fmt.Sprintf("CREATE DATABASE %s;", schemaName))
	require.NoError(t, err)
}

func relationsBlockData(t *testing.T, blockIdentifier string, blockTimeRaw string, entities ...*pbrelations.Entity) *pbsubstreamsrpc.Response {
	t.Helper()

	output := &pbrelations.Output{Entities: entities}

	return blockScopedData(t, blockIdentifier, output, blockTimepb(t, blockTimeRaw))
}

// entityCustomer creates a Customer entity with the given ID and name
func entityCustomer(customerId, name string) *pbrelations.Entity {
	return &pbrelations.Entity{
		Entity: &pbrelations.Entity_Customer{
			Customer: &pbrelations.Customer{
				CustomerId: customerId,
				Name:       name,
			},
		},
	}
}

// entityOrder creates an Order entity with the given order ID and customer reference ID
func entityOrder(orderId, customerRefId string, extension *pbrelations.OrderExtension, items ...*pbrelations.OrderItem) *pbrelations.Entity {
	return &pbrelations.Entity{
		Entity: &pbrelations.Entity_Order{
			Order: &pbrelations.Order{
				OrderId:       orderId,
				CustomerRefId: customerRefId,
				Extension:     extension,
				Items:         items,
			},
		},
	}
}

func orderItem(itemId string, quantity int64) *pbrelations.OrderItem {
	return &pbrelations.OrderItem{
		ItemId:   itemId,
		Quantity: quantity,
	}
}

func orderExtension(description string) *pbrelations.OrderExtension {
	return &pbrelations.OrderExtension{
		Description: description,
	}
}

func orderExtensionWithOptionalUint256(description string) *pbrelations.OrderExtension {
	ext := &pbrelations.OrderExtension{
		Description: description,
	}
	return ext
}

func entityOrderWithOptionalUint256(orderId, customerRefId string, extension *pbrelations.OrderExtension, items ...*pbrelations.OrderItem) *pbrelations.Entity {
	return &pbrelations.Entity{
		Entity: &pbrelations.Entity_Order{
			Order: &pbrelations.Order{
				OrderId:       orderId,
				CustomerRefId: customerRefId,
				Extension:     extension,
				Items:         items,
			},
		},
	}
}

// entityItem creates an Item entity with the given ID, name, and price
func entityItem(itemId, name string, price float64) *pbrelations.Entity {
	return &pbrelations.Entity{
		Entity: &pbrelations.Entity_Item{
			Item: &pbrelations.Item{
				ItemId: itemId,
				Name:   name,
				Price:  price,
			},
		},
	}
}

type BlockRow struct {
	Number    uint64    `db:"number"`
	Hash      string    `db:"hash"`
	Timestamp time.Time `db:"timestamp"`
}

type Meta struct {
	IsDeleted   bool      `db:"_deleted_"`
	BlockNumber uint64    `db:"_block_number_"`
	BlockTime   time.Time `db:"_block_timestamp_"`
}

func rowMeta(t *testing.T, blockNum uint, blockTimeRaw string) Meta {
	t.Helper()

	return Meta{
		IsDeleted:   false,
		BlockNumber: uint64(blockNum),
		BlockTime:   blockTime(t, blockTimeRaw),
	}
}

type OrderRow struct {
	Meta
	OrderID    string `db:"order_id"`
	CustomerID string `db:"customer_ref_id"`
}

// TypesTestRow represents a row from the types_tests table for assertions
type TypesTestRow struct {
	Meta
	ID uint64 `db:"id"`
}

// entityTypesTestWithEmptyRepeatedString creates a TypesTest entity with an empty
// repeated string field to reproduce the "cannot determine type of empty array" error.
func entityTypesTestWithEmptyRepeatedString(id uint64) *pbrelations.Entity {
	return &pbrelations.Entity{
		Entity: &pbrelations.Entity_TypesTest{
			TypesTest: &pbrelations.TypesTest{
				Id: id,
				// RepeatedStringField is intentionally left nil/empty to reproduce
				// the PostgreSQL error: "cannot determine type of empty array"
				RepeatedStringField: nil,
				// TimestampField must be set for ClickHouse to avoid panic on nil timestamp
				TimestampField: timestamppb.New(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)),
				// These fields have type conversions and require valid numeric strings (not empty)
				Str_2Int128:     "0",
				Str_2Uint128:    "0",
				Str_2Int256:     "0",
				Str_2Uint256:    "0",
				Str_2Decimal128: "0",
				Str_2Decimal256: "0",
				// Optional numeric conversion field - must be set to valid value for PostgreSQL
				// (empty string fails with "invalid input syntax for type numeric")
				OptionalStr_2Uint256: ptr("0"),
			},
		},
	}
}

func ptr[T any](v T) *T {
	return &v
}
