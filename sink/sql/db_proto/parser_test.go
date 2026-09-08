package db_proto

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"buf.build/go/hyperpb"
	sqlbytes "github.com/streamingfast/substreams/sink/sql/bytes"
	protosql "github.com/streamingfast/substreams/sink/sql/db_proto/sql"
	sqlpostgres "github.com/streamingfast/substreams/sink/sql/db_proto/sql/postgres"
	protoschema "github.com/streamingfast/substreams/sink/sql/db_proto/sql/schema"
	pbrelations "github.com/streamingfast/substreams/sink/sql/tests/relations"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// The sink only ever has the module's descriptor at runtime, never a generated Go type,
// so every block payload goes through a dynamic parser. This file compares the two that
// can do that job and pins the rules the faster one comes with.
//
// Everything here runs without a database: the parse needs only a descriptor, and the
// walk needs only a dialect and an in-memory inserter.

func outputDescriptor(t testing.TB) protoreflect.MessageDescriptor {
	t.Helper()

	descriptor := pbrelations.File_test_relations_relations_proto.Messages().ByName("Output")
	require.NotNil(t, descriptor)

	return descriptor
}

// payloadShapes are the two ends of what a module emits: a couple of columns per entity,
// and sixty-odd with arrays and every scalar type.
func payloadShapes(t testing.TB, entities int) []struct {
	name    string
	payload []byte
} {
	t.Helper()

	narrow, err := proto.Marshal(buildCustomerOutput(entities))
	require.NoError(t, err)

	wide, err := proto.Marshal(buildTypesTestOutput(entities))
	require.NoError(t, err)

	return []struct {
		name    string
		payload []byte
	}{
		{"narrow", narrow},
		{"wide", wide},
	}
}

// BenchmarkUnmarshal is the parse in isolation, which is what the switch to hyperpb is
// about: it compiles the descriptor into a parser once and then parses into an arena,
// where dynamicpb allocates its way through every field.
func BenchmarkUnmarshal(b *testing.B) {
	descriptor := outputDescriptor(b)
	messageType := hyperpb.CompileMessageDescriptor(descriptor)

	for _, shape := range payloadShapes(b, 200) {
		b.Run(shape.name+"/dynamicpb", func(b *testing.B) {
			b.SetBytes(int64(len(shape.payload)))
			b.ReportAllocs()

			for range b.N {
				message := dynamicpb.NewMessage(descriptor)
				if err := proto.Unmarshal(shape.payload, message); err != nil {
					b.Fatal(err)
				}
			}
		})

		b.Run(shape.name+"/hyperpb", func(b *testing.B) {
			b.SetBytes(int64(len(shape.payload)))
			b.ReportAllocs()

			shared := new(hyperpb.Shared)
			for range b.N {
				message := shared.NewMessage(messageType)
				if err := message.Unmarshal(shape.payload); err != nil {
					b.Fatal(err)
				}
				shared.Free()
			}
		})
	}
}

// BenchmarkDecodeBlock is the per-block work the decoder actually parallelises: parse,
// then walk the message and record the inserts. The parse is only part of it, so this is
// the ratio an operator sees rather than the headline one.
func BenchmarkDecodeBlock(b *testing.B) {
	logger := zap.NewNop()
	descriptor := outputDescriptor(b)

	schema, err := protoschema.NewSchema("bench", descriptor, true, logger)
	require.NoError(b, err)

	dialect, err := sqlpostgres.NewDialectPostgres(schema, sqlbytes.EncodingRaw, logger)
	require.NoError(b, err)

	base, err := protosql.NewBaseDatabase(string(descriptor.FullName()), descriptor, true, logger)
	require.NoError(b, err)

	messageType := hyperpb.CompileMessageDescriptor(descriptor)
	blockTime := time.Unix(1700000000, 0)

	for _, shape := range payloadShapes(b, 200) {
		b.Run(shape.name+"/dynamicpb", func(b *testing.B) {
			b.SetBytes(int64(len(shape.payload)))
			b.ReportAllocs()

			for range b.N {
				message := dynamicpb.NewMessage(descriptor)
				if err := proto.Unmarshal(shape.payload, message); err != nil {
					b.Fatal(err)
				}

				inserter := protosql.NewBufferedInserter(200)
				if _, err := base.WalkMessageDescriptorAndInsertWithDialect(message, 1, blockTime, nil, dialect, inserter); err != nil {
					b.Fatal(err)
				}
			}
		})

		b.Run(shape.name+"/hyperpb", func(b *testing.B) {
			b.SetBytes(int64(len(shape.payload)))
			b.ReportAllocs()

			shared := new(hyperpb.Shared)
			for range b.N {
				message := shared.NewMessage(messageType)
				if err := message.Unmarshal(shape.payload); err != nil {
					b.Fatal(err)
				}

				inserter := protosql.NewBufferedInserter(200)
				if _, err := base.WalkMessageDescriptorAndInsertWithDialect(message, 1, blockTime, nil, dialect, inserter); err != nil {
					b.Fatal(err)
				}
				shared.Free()
			}
		})
	}
}

// TestParsersAgree is the safety net for the swap: both parsers must produce the same
// message, read the way the walk reads it — through protoreflect only.
func TestParsersAgree(t *testing.T) {
	descriptor := outputDescriptor(t)
	messageType := hyperpb.CompileMessageDescriptor(descriptor)

	for _, shape := range payloadShapes(t, 20) {
		t.Run(shape.name, func(t *testing.T) {
			dynamic := dynamicpb.NewMessage(descriptor)
			require.NoError(t, proto.Unmarshal(shape.payload, dynamic))

			shared := new(hyperpb.Shared)
			defer shared.Free()

			hyper := shared.NewMessage(messageType)
			require.NoError(t, hyper.Unmarshal(shape.payload))

			marshal := protojson.MarshalOptions{Multiline: true, Indent: "  ", EmitUnpopulated: true}

			fromDynamic, err := marshal.Marshal(dynamic)
			require.NoError(t, err)

			fromHyper, err := marshal.Marshal(hyper)
			require.NoError(t, err)

			assert.JSONEq(t, string(fromDynamic), string(fromHyper))
		})
	}
}

// TestHyperpbArenaContract pins the two rules that come from hyperpb rather than from us,
// both of which are panics rather than errors, and both of which shape how the decoder
// keeps its arenas.
func TestHyperpbArenaContract(t *testing.T) {
	descriptor := outputDescriptor(t)
	messageType := hyperpb.CompileMessageDescriptor(descriptor)

	payload, err := proto.Marshal(buildCustomerOutput(4))
	require.NoError(t, err)

	t.Run("an arena carries one parse until freed", func(t *testing.T) {
		shared := new(hyperpb.Shared)
		defer shared.Free()

		message := shared.NewMessage(messageType)
		require.NoError(t, message.Unmarshal(payload))

		// Parsing again into the same arena is a panic, which is why the decoder keeps
		// one arena per block slot rather than one per worker.
		require.Panics(t, func() {
			second := shared.NewMessage(messageType)
			_ = second.Unmarshal(payload)
		})
	})

	t.Run("values stay valid until the arena is freed", func(t *testing.T) {
		shared := new(hyperpb.Shared)

		message := shared.NewMessage(messageType)
		require.NoError(t, message.Unmarshal(payload))

		entities := message.ProtoReflect().Descriptor().Fields().ByName("entities")
		require.NotNil(t, entities)

		list := message.ProtoReflect().Get(entities).List()
		require.Equal(t, 4, list.Len())

		// Strings point into the arena, so this is what the decoder must not free before
		// the walk's output has been replayed.
		first := list.Get(0).Message()
		customer := first.Descriptor().Fields().ByName("customer")
		require.NotNil(t, customer)
		name := first.Get(customer).Message()
		assert.Equal(t, "cust-00000000", name.Get(name.Descriptor().Fields().ByName("customer_id")).String())

		shared.Free()
	})
}

// TestHyperpbRejectsMutation fails loudly if a write is ever added to the walk's path:
// hyperpb messages are read-only, and the walk only ever reads.
func TestHyperpbRejectsMutation(t *testing.T) {
	descriptor := outputDescriptor(t)
	messageType := hyperpb.CompileMessageDescriptor(descriptor)

	payload, err := proto.Marshal(buildCustomerOutput(1))
	require.NoError(t, err)

	shared := new(hyperpb.Shared)
	defer shared.Free()

	message := shared.NewMessage(messageType)
	require.NoError(t, message.Unmarshal(payload))

	entities := message.ProtoReflect().Descriptor().Fields().ByName("entities")
	require.NotNil(t, entities)

	require.Panics(t, func() {
		message.ProtoReflect().Clear(entities)
	})
}

// TestHyperpbConcurrentParsesShareTheType covers the one thing the decoder really does
// share between goroutines: the compiled message type.
//
// The arena is not shared — there is one per block slot, and a slot is handed to exactly
// one worker per round — and it must not be, since it is an allocator with an explicit
// Free. What every worker touches at once is the *MessageType, which is safe because
// compiling is a one-time operation: hyperpb's own PGO path returns a new type from
// Recompile rather than mutating the one in use. Run with -race, this fails the day that
// stops being true.
func TestHyperpbConcurrentParsesShareTheType(t *testing.T) {
	descriptor := outputDescriptor(t)
	messageType := hyperpb.CompileMessageDescriptor(descriptor)

	payload, err := proto.Marshal(buildCustomerOutput(50))
	require.NoError(t, err)

	const workers = 8

	var wg sync.WaitGroup
	wg.Add(workers)
	for range workers {
		go func() {
			defer wg.Done()

			// One arena per goroutine, as the decoder gives one per block slot.
			shared := new(hyperpb.Shared)
			for range 50 {
				message := shared.NewMessage(messageType)
				if err := message.Unmarshal(payload); err != nil {
					t.Error(err)
					return
				}

				entities := message.ProtoReflect().Descriptor().Fields().ByName("entities")
				if got := message.ProtoReflect().Get(entities).List().Len(); got != 50 {
					t.Errorf("expected 50 entities, got %d", got)
					return
				}

				shared.Free()
			}
		}()
	}
	wg.Wait()
}

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
			}},
		}
	}

	return out
}
