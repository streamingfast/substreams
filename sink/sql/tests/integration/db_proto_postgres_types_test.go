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
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// TestDbProtoPostgresTypes runs every column type of the proto sink, with values that need care
// (quotes, backslashes, unicode, extremes, NaN, NULLs, empty arrays, unset messages), through
// every write path, and requires the stored rows to be the same.
func TestDbProtoPostgresTypes(t *testing.T) {
	outputMessageDescriptor := (*pbrelations.Output)(nil).ProtoReflect().Descriptor()

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
		TimestampField:        timestamppb.New(time.Date(2025, 1, 2, 3, 4, 5, 123456789, time.UTC)),
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
		Level1:                &pbrelations.NestedLevel1{Name: tricky, Desc: "d"},
		ListOfLevel1:          []*pbrelations.NestedLevel1{{Name: "a"}, {Name: tricky, Desc: "e"}},
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
	// Values that binary COPY stores and the rendered INSERTs used to reject, and fields
	// left unset that used to drop out of the row.
	edge := &pbrelations.TypesTest{
		Id:                   3,
		DoubleField:          math.NaN(),
		FloatField:           float32(math.Inf(1)),
		RepeatedDoubleField:  []float64{math.NaN(), math.Inf(1), math.Inf(-1)},
		RepeatedFloatField:   []float32{float32(math.Inf(-1))},
		OptionalStr_2Uint256: ptr(""),
	}

	results := runPostgresWritePaths(t, pbrelations.File_test_relations_relations_proto, outputMessageDescriptor, "types_tests", func(t *testing.T) []any {
		return []any{
			relationsBlockData(t, "1a", "2025-01-01T00:00:00.654321987Z", typesEntity(full)),
			relationsBlockData(t, "2a", "2025-01-02", typesEntity(minimal), typesEntity(edge)),
		}
	})

	copied := results[0]
	require.Contains(t, copied, `"timestamp_field":"2025-01-02T03:04:05.123456"`)
	require.Contains(t, copied, `"_block_timestamp_":"2025-01-01T00:00:00.654321"`)
	require.Contains(t, copied, `"double_field":"NaN","float_field":"Infinity"`)
	require.Contains(t, copied, `"repeated_double_field":["NaN","Infinity","-Infinity"]`)
	require.Contains(t, copied, `"str_2_int128":0,"str_2_uint128":0,"str_2_int256":0,"str_2_uint256":0,"str_2_decimal128":0.0000,"str_2_decimal256":0.0000,"optional_str_2_uint256":0`)
	require.Contains(t, copied, `"timestamp_field":null`)
	require.Contains(t, copied, `"level1":null,"list_of_level1":null`)
}

// TestDbProtoPostgresRepeatedNumericStrings covers repeated string fields converted to a
// NUMERIC[] column, including an empty element, which binary COPY stores as 0.
func TestDbProtoPostgresRepeatedNumericStrings(t *testing.T) {
	outputMessageDescriptor := (*pbrelations.PostgresValuesOutput)(nil).ProtoReflect().Descriptor()

	results := runPostgresWritePaths(t, pbrelations.File_test_relations_postgres_values_proto, outputMessageDescriptor, "postgres_values", func(t *testing.T) []any {
		return []any{
			blockScopedData(t, "1a", &pbrelations.PostgresValuesOutput{Values: []*pbrelations.PostgresValues{{
				Id:                      1,
				RepeatedStr_2Uint256:    []string{"115792089237316195423570985008687907853269984665640564039457584007913129639935", "", "1"},
				RepeatedStr_2Decimal128: []string{"12345.6789", ""},
			}}}, blockTimepb(t, "2025-01-01")),
			blockScopedData(t, "2a", &pbrelations.PostgresValuesOutput{Values: []*pbrelations.PostgresValues{{
				Id: 2,
			}}}, blockTimepb(t, "2025-01-02")),
		}
	})

	require.Contains(t, results[0], `"repeated_str_2_uint256":[115792089237316195423570985008687907853269984665640564039457584007913129639935,0,1],"repeated_str_2_decimal128":[12345.6789,0.0000]`)
	require.Contains(t, results[0], `"repeated_str_2_uint256":[],"repeated_str_2_decimal128":[]`)
}

// runPostgresWritePaths sinks blocks 1 and 2 through each way rows reach PostgreSQL, requires
// every path to store the same rows in table, and returns them, the binary COPY ones first.
//
// The spooled paths are what a backfill uses; the direct ones are what a sink switches to
// at the chain head, so a value only they mishandle stops a sink that has caught up.
func runPostgresWritePaths(t *testing.T, file protoreflect.FileDescriptor, outputMessageDescriptor protoreflect.MessageDescriptor, table string, blocks func(t *testing.T) []any) []string {
	t.Helper()

	postgresContainer := sharedDbChangesPostgresContainer
	ctx := context.Background()

	db, err := sql.Open("pgx", postgresContainer.ConnectionString)
	require.NoError(t, err)
	defer db.Close()

	paths := []struct {
		name  string
		mode  protosql.WriteMode
		spool bool
	}{
		{"copy", protosql.WriteModeCopy, true},
		{"spooled batch insert", protosql.WriteModeBatchInsert, true},
		{"spooled row insert", protosql.WriteModeRowInsert, true},
		{"direct batch insert", protosql.WriteModeBatchInsert, false},
		{"direct row insert", protosql.WriteModeRowInsert, false},
	}

	results := make([]string, len(paths))
	for i, path := range paths {
		t.Run(path.name, func(t *testing.T) {
			schemaName := table + "_" + strings.ReplaceAll(path.name, " ", "_")

			substreamsClientConfig := setupFakeSubstreamsServer(t, blocks(t)...)
			substreamsPackage := substreamsTestPackage(file, outputMessageDescriptor)
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
				WriteMode:       path.mode,
				DecodeBatchSize: 1,
			}.Defaults()
			if path.spool {
				options.Spool = &spool.Options{Dir: t.TempDir(), MaxIdle: 100_000_000}
			}

			createPostgresTestSchema(t, postgresContainer.ConnectionString, schemaName)
			dbSinker, err := db_proto.SinkerFactory(baseSink, defaultOutputModuleName, outputMessageDescriptor, options)(ctx, postgresContainer.ConnectionString, schemaName, logger, tracer)
			require.NoError(t, err)
			require.NoError(t, dbSinker.Run(ctx))
			require.NoError(t, dbSinker.Err())

			rows, err := db.Query(fmt.Sprintf(`SELECT row_to_json(t)::text FROM "%s"."%s" t ORDER BY id`, schemaName, table))
			require.NoError(t, err)
			defer rows.Close()
			var out strings.Builder
			for rows.Next() {
				var row string
				require.NoError(t, rows.Scan(&row))
				out.WriteString(row + "\n")
			}
			require.NoError(t, rows.Err())
			results[i] = out.String()
		})
	}

	for i, path := range paths[1:] {
		require.Equal(t, results[0], results[i+1], "copy and %s", path.name)
	}

	return results
}
