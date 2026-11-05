package tests

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/cenkalti/backoff/v4"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/streamingfast/bstream"
	"github.com/streamingfast/substreams/manifest"
	sink "github.com/streamingfast/substreams/sink"
	"github.com/streamingfast/substreams/sink/sql/db_proto"
	protosql "github.com/streamingfast/substreams/sink/sql/db_proto/sql"
	"github.com/streamingfast/substreams/sink/sql/db_proto/sql/spool"
	pbrelations "github.com/streamingfast/substreams/sink/sql/tests/relations"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// TestDbProtoPostgresTypes runs every column type of the proto sink, with values that need care
// (quotes, backslashes, unicode, extremes, NULLs, empty arrays), through every write mode, and
// requires the stored rows to be the same.
func TestDbProtoPostgresTypes(t *testing.T) {
	outputMessageDescriptor := (*pbrelations.Output)(nil).ProtoReflect().Descriptor()
	postgresContainer := sharedDbChangesPostgresContainer
	ctx := context.Background()

	typesEntity := func(types *pbrelations.TypesTest) *pbrelations.Entity {
		return &pbrelations.Entity{Entity: &pbrelations.Entity_TypesTest{TypesTest: types}}
	}
	tricky := `quote ' double " back\slash, comma {brace} été 日本語`
	full := &pbrelations.TypesTest{
		Id:                    1,
		DoubleField:           math.Pi,
		FloatField:            1.5,
		Int32Field:            math.MinInt32,
		Int64Field:            math.MinInt64,
		Uint32Field:           math.MaxUint32,
		Uint64Field:           math.MaxInt64,
		Sint32Field:           -32,
		Sint64Field:           -64,
		Fixed32Field:          32,
		Fixed64Field:          64,
		Sfixed32Field:         -320,
		Sfixed64Field:         -640,
		BoolField:             true,
		StringField:           tricky,
		BytesField:            []byte{0x00, 0x01, 0xfe, 0xff, '\'', '\\'},
		OptionalStringSet:     ptr(tricky),
		OptionalInt32FieldSet: ptr(int32(7)),
		TimestampField:        timestamppb.New(time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)),
		RepeatedInt32Field:    []int32{1, -2},
		RepeatedInt64Field:    []int64{math.MaxInt64, math.MinInt64},
		RepeatedUint32Field:   []uint32{math.MaxUint32},
		RepeatedDoubleField:   []float64{1.25, -2.5},
		RepeatedFloatField:    []float32{0.5},
		RepeatedBoolField:     []bool{true, false},
		RepeatedStringField:   []string{"plain", tricky, "", "NULL"},
		Str_2Int128:           "-170141183460469231731687303715884105728",
		Str_2Uint128:          "340282366920938463463374607431768211455",
		Str_2Int256:           "-57896044618658097711785492504343953926634992332820282019728792003956564819968",
		Str_2Uint256:          "115792089237316195423570985008687907853269984665640564039457584007913129639935",
		Str_2Decimal128:       "12345.6789",
		Str_2Decimal256:       "-98765.4321",
		OptionalStr_2Uint256:  ptr("1"),
	}
	minimal := &pbrelations.TypesTest{
		Id:                   2,
		BytesField:           []byte{0x00},
		StringField:          "",
		TimestampField:       timestamppb.New(time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)),
		Str_2Int128:          "0",
		Str_2Uint128:         "0",
		Str_2Int256:          "0",
		Str_2Uint256:         "0",
		Str_2Decimal128:      "0",
		Str_2Decimal256:      "0",
		OptionalStr_2Uint256: ptr("0"),
	}

	db, err := sql.Open("pgx", postgresContainer.ConnectionString)
	require.NoError(t, err)
	defer db.Close()

	run := func(t *testing.T, schemaName string, mode protosql.WriteMode) string {
		t.Helper()

		substreamsClientConfig := setupFakeSubstreamsServer(t,
			relationsBlockData(t, "1a", "2025-01-01", typesEntity(full)),
			relationsBlockData(t, "2a", "2025-01-02", typesEntity(minimal)),
		)
		substreamsPackage := substreamsTestPackage(pbrelations.File_test_relations_relations_proto, outputMessageDescriptor)
		baseSink, err := sink.New(sink.SubstreamsModeProduction, false, substreamsPackage, substreamsPackage.Modules.Modules[0], manifest.ModuleHash{},
			substreamsClientConfig, logger, tracer,
			sink.WithBlockRange(bstream.MustParseRange("1-3", bstream.WithExclusiveEnd())),
			sink.WithRetryBackOff(&backoff.StopBackOff{}),
		)
		require.NoError(t, err)

		options := db_proto.SinkerFactoryOptions{
			UseProtoOption:  true,
			Constraints:     protosql.ConstraintPolicy{Timing: protosql.ConstraintsManual},
			UseTransactions: true,
			WriteMode:       mode,
			DecodeBatchSize: 1,
			Spool:           &spool.Options{Dir: t.TempDir(), MaxIdle: 100_000_000},
		}.Defaults()

		createPostgresTestSchema(t, postgresContainer.ConnectionString, schemaName)
		dbSinker, err := db_proto.SinkerFactory(baseSink, defaultOutputModuleName, outputMessageDescriptor, options)(ctx, postgresContainer.ConnectionString, schemaName, logger, tracer)
		require.NoError(t, err)
		require.NoError(t, dbSinker.Run(ctx))
		require.NoError(t, dbSinker.Err())

		rows, err := db.Query(fmt.Sprintf(`SELECT row_to_json(t)::text FROM "%s"."types_tests" t ORDER BY id`, schemaName))
		require.NoError(t, err)
		defer rows.Close()
		var out strings.Builder
		for rows.Next() {
			var row string
			require.NoError(t, rows.Scan(&row))
			out.WriteString(row + "\n")
		}
		require.NoError(t, rows.Err())
		return out.String()
	}

	var results []string
	for _, mode := range []protosql.WriteMode{protosql.WriteModeCopy, protosql.WriteModeBatchInsert, protosql.WriteModeRowInsert} {
		t.Run(string(mode), func(t *testing.T) {
			results = append(results, run(t, "types_"+strings.ReplaceAll(string(mode), "-", "_"), mode))
		})
	}
	require.Len(t, results, 3)
	require.Equal(t, results[0], results[1], "copy and batch insert")
	require.Equal(t, results[0], results[2], "copy and row insert")
}
