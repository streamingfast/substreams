package tests

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	"github.com/streamingfast/bstream"
	"github.com/streamingfast/substreams/manifest"
	sink "github.com/streamingfast/substreams/sink"
	sqlbytes "github.com/streamingfast/substreams/sink/sql/bytes"
	"github.com/streamingfast/substreams/sink/sql/db_proto"
	pbrelations "github.com/streamingfast/substreams/sink/sql/tests/relations"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// TestDbProtoPostgresBytesRoundTrip pins what actually lands in a BYTEA column, for both
// inserter implementations.
//
// The two paths encode values completely differently — AccumulatorInserter renders SQL
// literals through ValueToString, RowInserter binds parameters to a prepared statement —
// and neither was covered for bytes. A byte slice that does not survive the round trip is
// silent data corruption: the rows are all there and every query against them misses.
func TestDbProtoPostgresBytesRoundTrip(t *testing.T) {
	payload := []byte{0xde, 0xad, 0xbe, 0xef, 0x00, 0x7f, 0xff}
	expectedTimestamp := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	for _, useConstraints := range []bool{false, true} {
		name := "accumulator_inserter"
		if useConstraints {
			name = "row_inserter"
		}

		t.Run(name, func(t *testing.T) {
			schema := "bytes_roundtrip_" + name
			dbx := runBytesSinker(t, schema, useConstraints, payload, expectedTimestamp)
			defer dbx.Close()

			var (
				stored    []byte
				timestamp time.Time
				topics    pq.StringArray
			)
			err := dbx.QueryRow(fmt.Sprintf(
				`SELECT bytes_field, timestamp_field, repeated_string_field FROM %q.types_tests`, schema,
			)).Scan(&stored, &timestamp, &topics)
			require.NoError(t, err)

			assert.Equal(t, payload, stored,
				"bytes_field did not survive the round trip through the %s", name)
			assert.True(t, expectedTimestamp.Equal(timestamp),
				"timestamp_field did not survive the round trip through the %s: expected %s, got %s",
				name, expectedTimestamp, timestamp)
			assert.Equal(t, []string{"alpha", "beta"}, []string(topics),
				"repeated_string_field did not survive the round trip through the %s", name)
		})
	}
}

// runBytesSinker streams a single TypesTest entity carrying bytes_field through the real
// sinker and returns a handle on the resulting database.
func runBytesSinker(t *testing.T, schema string, useConstraints bool, payload []byte, timestamp time.Time) *sqlx.DB {
	t.Helper()

	outputMessageDescriptor := (*pbrelations.Output)(nil).ProtoReflect().Descriptor()
	postgresContainer := sharedDbChangesPostgresContainer

	entity := &pbrelations.Entity{
		Entity: &pbrelations.Entity_TypesTest{
			TypesTest: &pbrelations.TypesTest{
				Id:                   1,
				BytesField:           payload,
				TimestampField:       timestamppb.New(timestamp),
				RepeatedStringField:  []string{"alpha", "beta"},
				Str_2Int128:          "0",
				Str_2Uint128:         "0",
				Str_2Int256:          "0",
				Str_2Uint256:         "0",
				Str_2Decimal128:      "0",
				Str_2Decimal256:      "0",
				OptionalStr_2Uint256: ptr("0"),
			},
		},
	}

	substreamsClientConfig := setupFakeSubstreamsServer(t,
		relationsBlockData(t, "1a", "2025-01-01", entity))
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
		UseConstraints:  useConstraints,
		UseTransactions: true,
		BlockBatchSize:  1,
		Parallel:        false,
		Encoding:        sqlbytes.EncodingRaw,
	}.Defaults()
	options.Encoding = sqlbytes.EncodingRaw

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
