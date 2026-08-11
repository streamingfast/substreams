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
	pbrelations "github.com/streamingfast/substreams/sink/sql/tests/relations"
	"github.com/stretchr/testify/require"
)

func TestDbProtoPostgresIntegration(t *testing.T) {
	outputMessageDescriptor := (*pbrelations.Output)(nil).ProtoReflect().Descriptor()

	postgresContainer := sharedDbChangesPostgresContainer

	streamMock := func(responses ...*pbsubstreamsrpc.Response) []*pbsubstreamsrpc.Response {
		return responses
	}

	equalsTypesTestRows := func(expected []*TypesTestRow) func(t *testing.T, dbx *sqlx.DB, schema string) {
		return func(t *testing.T, dbx *sqlx.DB, schema string) {
			rows := readRowsBy[TypesTestRow](t, dbx, fmt.Sprintf(`"%s"."types_tests"`, schema), "id")
			require.Len(t, rows, len(expected))
			for i, exp := range expected {
				actual := rows[i]
				// Compare time values using Equal() to ignore timezone location differences
				require.True(t, exp.BlockTime.Equal(actual.BlockTime), "BlockTime mismatch at index %d: expected %v, got %v", i, exp.BlockTime, actual.BlockTime)
				require.Equal(t, exp.IsDeleted, actual.IsDeleted)
				require.Equal(t, exp.BlockNumber, actual.BlockNumber)
				require.Equal(t, exp.ID, actual.ID)
			}
		}
	}

	testCases := []struct {
		name      string
		responses []*pbsubstreamsrpc.Response
		expected  func(t *testing.T, dbx *sqlx.DB, schema string)
	}{
		{
			// This test case reproduces the issue where PostgreSQL cannot determine the type
			// of an empty array. When a repeated field is empty (nil or empty slice), the
			// generated INSERT statement uses '{}' for the array, but PostgreSQL doesn't know
			// what type to use for the empty array.
			//
			// Error: pq: cannot determine type of empty array
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

			options := db_proto.SinkerFactoryOptions{
				UseProtoOption:  true,
				UseConstraints:  false,
				UseTransactions: true,
				BlockBatchSize:  1,
			}.Defaults()

			sinkerFactory := db_proto.SinkerFactory(
				baseSink,
				defaultOutputModuleName,
				outputMessageDescriptor,
				options,
			)

			testSchema := strings.ReplaceAll(strings.ToLower(tc.name), " ", "_")
			createPostgresTestSchema(t, postgresContainer.ConnectionString, testSchema)

			ctx := context.Background()
			dbSinker, err := sinkerFactory(ctx, postgresContainer.ConnectionString, testSchema, logger, tracer)
			require.NoError(t, err)

			err = dbSinker.Run(ctx)
			require.NoError(t, err)
			require.NoError(t, dbSinker.Err())

			db, err := sql.Open("postgres", postgresContainer.ConnectionString)
			require.NoError(t, err)

			dbx := sqlx.NewDb(db, "postgres").Unsafe()
			defer dbx.Close()

			if tc.expected != nil {
				tc.expected(t, dbx, testSchema)
			}
		})
	}
}

func createPostgresTestSchema(t *testing.T, dsn string, schemaName string) {
	t.Helper()

	db, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	defer db.Close()

	_, err = db.Exec(fmt.Sprintf(`CREATE SCHEMA IF NOT EXISTS "%s";`, schemaName))
	require.NoError(t, err)
}
