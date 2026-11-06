package benchmarks

import (
	"fmt"
	"io"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	sqlbytes "github.com/streamingfast/substreams/sink/sql/bytes"
	protosql "github.com/streamingfast/substreams/sink/sql/db_proto/sql"
	sqlpostgres "github.com/streamingfast/substreams/sink/sql/db_proto/sql/postgres"
	"github.com/streamingfast/substreams/sink/sql/db_proto/sql/postgres/pgcopy"
	protoschema "github.com/streamingfast/substreams/sink/sql/db_proto/sql/schema"
	pbrelations "github.com/streamingfast/substreams/sink/sql/tests/relations"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/dynamicpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// TestClientEncodeCeiling measures how fast the sink can turn a substreams payload into
// something loadable, with no database involved at all.
//
// This is the number that decides whether decoupling the stream from PostgreSQL is
// worth anything. COPY BINARY absorbs ~800k rows/s (see TestCopyVsInsert); if the
// client cannot produce rows at that rate then PostgreSQL was never the bottleneck and
// the work belongs on this side instead.
//
// It runs the production path: proto.Unmarshal into dynamicpb, then
// BaseDatabase.WalkMessageDescriptorAndInsertWithDialect with the real Postgres
// dialect. Only the Inserter changes between variants.
//
//	go test ./sink/sql/db_proto/benchmarks/ -run TestClientEncodeCeiling -v
func TestClientEncodeCeiling(t *testing.T) {
	requireBenchmark(t)

	shapes := []struct {
		name        string
		entities    int
		build       func(count int) *pbrelations.Output
		rowsPerItem int
	}{
		{
			name:        "narrow-entity (2 columns)",
			entities:    200,
			build:       buildCustomerOutput,
			rowsPerItem: 1,
		},
		{
			name:        "wide-entity (60+ columns, arrays, inline JSONB)",
			entities:    200,
			build:       buildTypesTestOutput,
			rowsPerItem: 1,
		},
	}

	logger := zap.NewNop()
	descriptor := pbrelations.File_test_relations_relations_proto.Messages().ByName("Output")
	require.NotNil(t, descriptor)

	schema, err := protoschema.NewSchema("bench", descriptor, true, logger)
	require.NoError(t, err)

	dialect, err := sqlpostgres.NewDialectPostgres(schema, sqlbytes.EncodingRaw, logger)
	require.NoError(t, err)

	base, err := protosql.NewBaseDatabase(string(descriptor.FullName()), descriptor, true, logger)
	require.NoError(t, err)

	for _, shape := range shapes {
		payload, err := proto.Marshal(shape.build(shape.entities))
		require.NoError(t, err)

		rowsPerPayload := shape.entities * shape.rowsPerItem

		variants := []struct {
			name     string
			notes    string
			inserter func() protosql.Inserter
			decode   bool
		}{
			{
				name:  "unmarshal-only",
				notes: "proto.Unmarshal into dynamicpb, no walk",
			},
			{
				name:     "unmarshal+walk-discard",
				notes:    "walk the message, throw the values away: the floor for any strategy",
				inserter: func() protosql.Inserter { return &discardInserter{} },
			},
			{
				name:     "unmarshal+walk+pgcopy-binary",
				notes:    "walk, then encode straight to the binary COPY format",
				inserter: func() protosql.Inserter { return newPgcopyInserter(inferOID) },
			},
			{
				name:  "unmarshal+walk+pgcopy-binary uint32->BIGINT",
				notes: "same, but uint32/fixed32 map to BIGINT instead of NUMERIC",
				inserter: func() protosql.Inserter {
					return newPgcopyInserter(func(v any) uint32 {
						switch v.(type) {
						case uint32:
							return pgtype.Int8OID
						default:
							return inferOID(v)
						}
					})
				},
			},
			{
				name:  "unmarshal+walk+pgcopy-binary no-NUMERIC",
				notes: "upper bound: every unsigned int as BIGINT, isolating the numeric codec's cost",
				inserter: func() protosql.Inserter {
					return newPgcopyInserter(func(v any) uint32 {
						switch v.(type) {
						case uint32, uint64, uint:
							return pgtype.Int8OID
						default:
							return inferOID(v)
						}
					})
				},
			},
			{
				name:     "unmarshal+walk+text-literal",
				notes:    "walk, then ValueToString into a VALUES buffer (what happens today)",
				inserter: func() protosql.Inserter { return newTextInserter() },
			},
		}

		results := make([]clientResult, 0, len(variants))
		for _, v := range variants {
			var inserter protosql.Inserter
			if v.inserter != nil {
				inserter = v.inserter()
			}

			iterations, elapsed := runUntil(2*time.Second, func() {
				message := dynamicpb.NewMessage(descriptor)
				if err := proto.Unmarshal(payload, message); err != nil {
					t.Fatalf("unmarshal: %v", err)
				}
				if inserter == nil {
					return
				}
				if _, err := base.WalkMessageDescriptorAndInsertWithDialect(
					message, 20_000_000, time.Unix(1700000000, 0).UTC(), nil, dialect, inserter,
				); err != nil {
					t.Fatalf("walk: %v", err)
				}
			})

			results = append(results, clientResult{
				name:     v.name,
				notes:    v.notes,
				rowsPerS: float64(iterations*rowsPerPayload) / elapsed.Seconds(),
				mibPerS:  float64(iterations*len(payload)) / elapsed.Seconds() / (1024 * 1024),
			})
		}

		reportClient(t, shape.name, len(payload), rowsPerPayload, results)
	}
}

type clientResult struct {
	name     string
	notes    string
	rowsPerS float64
	mibPerS  float64
}

func runUntil(budget time.Duration, fn func()) (iterations int, elapsed time.Duration) {
	start := time.Now()
	for elapsed < budget {
		fn()
		iterations++
		if iterations%16 == 0 {
			elapsed = time.Since(start)
		}
	}

	return iterations, time.Since(start)
}

func reportClient(t *testing.T, shape string, payloadBytes, rowsPerPayload int, results []clientResult) {
	t.Helper()

	var b strings.Builder
	fmt.Fprintf(&b, "\n\n%s -- %d rows per payload, %s of protobuf\n\n", shape, rowsPerPayload, humanBytes(int64(payloadBytes)))
	fmt.Fprintf(&b, "%-32s %12s %10s  %s\n", "variant", "rows/s", "MiB/s", "notes")
	for _, r := range results {
		fmt.Fprintf(&b, "%-32s %12s %10.1f  %s\n", r.name, humanCount(r.rowsPerS), r.mibPerS, r.notes)
	}

	t.Log(b.String())
}

// -- inserters ---------------------------------------------------------------------

// discardInserter measures the walk itself: dynamicpb reflection, the per-message
// zap.Any debug fields, and the []any building.
type discardInserter struct{ rows int64 }

func (i *discardInserter) Insert(table string, values []any) error {
	i.rows++
	return nil
}

// pgcopyInserter encodes into the binary COPY format and throws the bytes away.
//
// Column OIDs are inferred from the first row's Go types rather than read from
// pg_attribute, since there is no server here. The encoding work per value is
// identical; only the one-time OID resolution differs from production.
type pgcopyInserter struct {
	targets map[string]*pgcopyTarget
	oidOf   func(any) uint32
}

type pgcopyTarget struct {
	writer  *pgcopy.Writer
	columns []pgcopy.Column
}

func newPgcopyInserter(oidOf func(any) uint32) *pgcopyInserter {
	return &pgcopyInserter{targets: map[string]*pgcopyTarget{}, oidOf: oidOf}
}

func (i *pgcopyInserter) Insert(table string, values []any) error {
	target, ok := i.targets[table]
	if !ok {
		columns := make([]pgcopy.Column, len(values))
		for j, v := range values {
			columns[j] = pgcopy.Column{Name: fmt.Sprintf("c%d", j), OID: i.oidOf(v)}
		}

		writer, err := pgcopy.NewWriter(io.Discard, columns)
		if err != nil {
			return err
		}
		target = &pgcopyTarget{writer: writer, columns: columns}
		i.targets[table] = target
	}

	if len(values) != len(target.columns) {
		return fmt.Errorf("table %s: expected %d values, got %d", table, len(target.columns), len(values))
	}
	if err := pgcopy.NormalizeRow(target.columns, values); err != nil {
		return fmt.Errorf("normalizing %s: %w", table, err)
	}

	return target.writer.WriteRow(values)
}

// textInserter is the current AccumulatorInserter client-side work: stringify every
// value and append it to a VALUES buffer.
type textInserter struct {
	buffers map[string]*strings.Builder
}

func newTextInserter() *textInserter {
	return &textInserter{buffers: map[string]*strings.Builder{}}
}

func (i *textInserter) Insert(table string, values []any) error {
	buffer, ok := i.buffers[table]
	if !ok {
		buffer = &strings.Builder{}
		i.buffers[table] = buffer
	}

	buffer.WriteByte('(')
	for j, v := range values {
		if j > 0 {
			buffer.WriteByte(',')
		}
		buffer.WriteString(sqlpostgres.ValueToString(v, sqlbytes.EncodingRaw))
	}
	buffer.WriteString("),")

	// A flush would hand this off and reset; keep the buffer bounded so the benchmark
	// measures encoding rather than the allocator growing a multi-gigabyte string.
	if buffer.Len() > 8<<20 {
		buffer.Reset()
	}

	return nil
}

func inferOID(value any) uint32 {
	switch v := value.(type) {
	case nil:
		return pgtype.TextOID
	case bool:
		return pgtype.BoolOID
	case int32:
		return pgtype.Int4OID
	case int64:
		return pgtype.Int8OID
	case uint32, uint64, uint:
		return pgtype.NumericOID
	case float32:
		return pgtype.Float4OID
	case float64:
		return pgtype.Float8OID
	case string:
		return pgtype.TextOID
	case []byte:
		return pgtype.ByteaOID
	case time.Time, *timestamppb.Timestamp:
		return pgtype.TimestampOID
	case []any:
		if len(v) == 0 {
			return pgtype.TextArrayOID
		}
		return arrayOIDFor(inferOID(v[0]))
	default:
		return pgtype.JSONBOID
	}
}

func arrayOIDFor(element uint32) uint32 {
	switch element {
	case pgtype.BoolOID:
		return pgtype.BoolArrayOID
	case pgtype.Int4OID:
		return pgtype.Int4ArrayOID
	case pgtype.Int8OID:
		return pgtype.Int8ArrayOID
	case pgtype.NumericOID:
		return pgtype.NumericArrayOID
	case pgtype.Float4OID:
		return pgtype.Float4ArrayOID
	case pgtype.Float8OID:
		return pgtype.Float8ArrayOID
	case pgtype.ByteaOID:
		return pgtype.ByteaArrayOID
	default:
		return pgtype.TextArrayOID
	}
}

// -- payload builders --------------------------------------------------------------

func buildCustomerOutput(count int) *pbrelations.Output {
	out := &pbrelations.Output{Entities: make([]*pbrelations.Entity, count)}
	for i := range out.Entities {
		out.Entities[i] = &pbrelations.Entity{
			Entity: &pbrelations.Entity_Customer{Customer: &pbrelations.Customer{
				CustomerId: fmt.Sprintf("cust-%08d", i),
				Name:       fmt.Sprintf("Customer Number %d", i),
			}},
		}
	}

	return out
}

func buildTypesTestOutput(count int) *pbrelations.Output {
	out := &pbrelations.Output{Entities: make([]*pbrelations.Entity, count)}
	optionalString := "set"
	optionalInt := int32(42)

	for i := range out.Entities {
		out.Entities[i] = &pbrelations.Entity{
			Entity: &pbrelations.Entity_TypesTest{TypesTest: &pbrelations.TypesTest{
				Id:                    uint64(i),
				DoubleField:           float64(i) * 1.5,
				FloatField:            float32(i) * 2.5,
				Int32Field:            int32(i),
				Int64Field:            int64(i) * 1000,
				Uint32Field:           uint32(i),
				Uint64Field:           uint64(i) * 7,
				Sint32Field:           int32(-i),
				Sint64Field:           int64(-i) * 3,
				Fixed32Field:          uint32(i),
				Fixed64Field:          uint64(i),
				Sfixed32Field:         int32(i),
				Sfixed64Field:         int64(i),
				BoolField:             i%2 == 0,
				StringField:           fmt.Sprintf("string value %d", i),
				BytesField:            []byte(fmt.Sprintf("bytes-%d", i)),
				OptionalStringSet:     &optionalString,
				OptionalInt32FieldSet: &optionalInt,
				TimestampField:        timestamppb.New(time.Unix(1700000000+int64(i), 0)),
				RepeatedInt32Field:    []int32{1, 2, 3},
				RepeatedInt64Field:    []int64{4, 5, 6},
				RepeatedUint32Field:   []uint32{7, 8},
				RepeatedUint64Field:   []uint64{9, 10},
				RepeatedStringField:   []string{"alpha", "beta", "gamma"},
				RepeatedBoolField:     []bool{true, false},
				RepeatedDoubleField:   []float64{1.1, 2.2},
				Str_2Int128:           "-170141183460469231731687303715884105728",
				Str_2Uint128:          "340282366920938463463374607431768211455",
				Str_2Int256:           "-57896044618658097711785492504343953926634992332820282019728792003956564819968",
				Str_2Uint256:          "115792089237316195423570985008687907853269984665640564039457584007913129639935",
				Str_2Decimal128:       "1234.5678",
				Str_2Decimal256:       "8765.4321",
				// The inline nested fields (level1, list_of_level1) exist in the .proto
				// but not in the checked-in relations.pb.go, so the inline-JSONB path is
				// not covered here.
			}},
		}
	}

	return out
}

// TestClientDecodeScaling measures how the per-block work the decoder parallelises —
// unmarshal, walk, and buffering the inserts — scales across cores.
//
// It is the same workload sink/sql/db_proto's decoder runs per block, so it bounds what
// the worker pool can achieve. The replay of the buffered inserts is deliberately not
// included: that part stays serial on the flush goroutine.
func TestClientDecodeScaling(t *testing.T) {
	requireBenchmark(t)

	logger := zap.NewNop()
	descriptor := pbrelations.File_test_relations_relations_proto.Messages().ByName("Output")
	require.NotNil(t, descriptor)

	schema, err := protoschema.NewSchema("bench", descriptor, true, logger)
	require.NoError(t, err)

	dialect, err := sqlpostgres.NewDialectPostgres(schema, sqlbytes.EncodingRaw, logger)
	require.NoError(t, err)

	base, err := protosql.NewBaseDatabase(string(descriptor.FullName()), descriptor, true, logger)
	require.NoError(t, err)

	const entities = 200
	payload, err := proto.Marshal(buildTypesTestOutput(entities))
	require.NoError(t, err)

	decodeOne := func() {
		message := dynamicpb.NewMessage(descriptor)
		if err := proto.Unmarshal(payload, message); err != nil {
			panic(err)
		}
		buffer := protosql.NewBufferedInserter(entities)
		if _, err := base.WalkMessageDescriptorAndInsertWithDialect(
			message, 20_000_000, time.Unix(1700000000, 0).UTC(), nil, dialect, buffer,
		); err != nil {
			panic(err)
		}
	}

	var b strings.Builder
	fmt.Fprintf(&b, "\n\nwide entity, %d rows per block, decode work only (%d cores available)\n\n", entities, runtime.NumCPU())
	fmt.Fprintf(&b, "%-10s %12s %10s\n", "workers", "rows/s", "speedup")

	var serial float64
	for _, workers := range []int{1, 2, 4, 8, max(1, runtime.NumCPU()-1)} {
		blocks, elapsed := runUntilParallel(2*time.Second, workers, decodeOne)
		rowsPerSecond := float64(blocks*entities) / elapsed.Seconds()
		if workers == 1 {
			serial = rowsPerSecond
		}
		fmt.Fprintf(&b, "%-10d %12s %9.2fx\n", workers, humanCount(rowsPerSecond), rowsPerSecond/serial)
	}

	t.Log(b.String())
}

// runUntilParallel runs fn on the given number of goroutines until the budget elapses,
// returning the total number of calls completed.
func runUntilParallel(budget time.Duration, workers int, fn func()) (calls int, elapsed time.Duration) {
	var (
		wg    sync.WaitGroup
		total atomic.Int64
		start = time.Now()
	)

	wg.Add(workers)
	for range workers {
		go func() {
			defer wg.Done()
			local := 0
			for time.Since(start) < budget {
				for range 16 {
					fn()
				}
				local += 16
			}
			total.Add(int64(local))
		}()
	}
	wg.Wait()

	return int(total.Load()), time.Since(start)
}
