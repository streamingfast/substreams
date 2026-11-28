package rows

import (
	"fmt"
	"testing"
	"time"

	"buf.build/go/hyperpb"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"

	dbsql "github.com/streamingfast/substreams/sink/sql/db_proto/sql"
	dbschema "github.com/streamingfast/substreams/sink/sql/db_proto/sql/schema"
	"github.com/streamingfast/substreams/sink/sql/tests/relations"
)

// wideOutput: 1000 types_tests rows, scalar heavy.
func wideOutput() *relations.Output {
	out := &relations.Output{}
	for i := range 1000 {
		out.Entities = append(out.Entities, &relations.Entity{Entity: &relations.Entity_TypesTest{TypesTest: &relations.TypesTest{
			Id: uint64(i), DoubleField: 1.5, Int32Field: int32(i), StringField: "hello", BytesField: []byte{1, 2, 3},
			OptionalStringSet: ptr("x"), RepeatedInt32Field: []int32{1, 2, 3}, Str_2Uint256: "123456789012345678901234567890",
			Str_2Decimal128: "12.3456", Level1: &relations.NestedLevel1{Name: "n"},
		}}})
	}
	return out
}

// treeOutput: 100 orders with 100 items each (10k child rows).
func treeOutput() *relations.Output {
	out := &relations.Output{}
	for i := range 100 {
		order := &relations.Order{OrderId: fmt.Sprintf("o%d", i), CustomerRefId: "c", Extension: &relations.OrderExtension{Description: "d"}}
		for j := range 100 {
			order.Items = append(order.Items, &relations.OrderItem{ItemId: fmt.Sprintf("i%d", j), Quantity: int64(j)})
		}
		out.Entities = append(out.Entities, &relations.Entity{Entity: &relations.Entity_Order{Order: order}})
	}
	return out
}

type benchInput struct {
	name string
	out  *relations.Output
}

var benchInputs = []benchInput{{"wide", wideOutput()}, {"tree", treeOutput()}}

func benchProducer(b *testing.B, opts ...Option) *SchemaRowProducer {
	b.Helper()

	p, err := NewSchemaRowProducer(relationsDescriptor(), opts...)
	if err != nil {
		b.Fatal(err)
	}
	return p
}

func parsed(b *testing.B, in *relations.Output) map[string]protoreflect.Message {
	b.Helper()

	payload, err := proto.Marshal(in)
	if err != nil {
		b.Fatal(err)
	}

	dynamic := dynamicpb.NewMessage(relationsDescriptor())
	if err := proto.Unmarshal(payload, dynamic); err != nil {
		b.Fatal(err)
	}

	hyper := new(hyperpb.Shared).NewMessage(hyperpb.CompileMessageDescriptor(relationsDescriptor()))
	if err := hyper.Unmarshal(payload); err != nil {
		b.Fatal(err)
	}

	return map[string]protoreflect.Message{"dynamicpb": dynamic, "hyperpb": hyper.ProtoReflect()}
}

func BenchmarkWalk(b *testing.B) {
	for _, input := range benchInputs {
		messages := parsed(b, input.out)
		for _, impl := range []string{"dynamicpb", "hyperpb"} {
			for _, check := range []bool{false, true} {
				name := fmt.Sprintf("%s/%s/check=%t", input.name, impl, check)
				b.Run(name, func(b *testing.B) {
					var opts []Option
					if check {
						opts = append(opts, WithCheck())
					}
					p := benchProducer(b, opts...)
					msg := messages[impl]

					b.ReportAllocs()
					rows := 0
					for b.Loop() {
						for row, err := range p.Rows(testBlock, msg) {
							if err != nil {
								b.Fatal(err)
							}
							rows += len(row.Values)
						}
					}
					_ = rows
				})
			}
		}
	}
}

type discardInserter struct{ rows int }

func (d *discardInserter) Insert(string, []any) error { d.rows++; return nil }

// BenchmarkBaselineDBProtoWalk is the existing db_proto walk with a discarding inserter, on the same messages.
func BenchmarkBaselineDBProtoWalk(b *testing.B) {
	schema, err := dbschema.NewSchema("bench", relationsDescriptor(), true, zap.NewNop())
	if err != nil {
		b.Fatal(err)
	}
	database, err := dbsql.NewBaseDatabase("bench", relationsDescriptor(), true, zap.NewNop())
	if err != nil {
		b.Fatal(err)
	}
	dialect := registryDialect{tables: schema.TableRegistry}

	for _, input := range benchInputs {
		messages := parsed(b, input.out)
		for _, impl := range []string{"dynamicpb", "hyperpb"} {
			b.Run(input.name+"/"+impl, func(b *testing.B) {
				msg := messages[impl]
				inserter := &discardInserter{}

				b.ReportAllocs()
				for b.Loop() {
					if _, err := database.WalkMessageDescriptorAndInsertWithDialect(msg, 100, time.Unix(0, 0), nil, dialect, inserter); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}

func BenchmarkCheck(b *testing.B) {
	b.Run("bigint256", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			_ = CheckBigInt("115792089237316195423570985008687907853269984665640564039457584007913129639935", false, 256)
		}
	})
	b.Run("decimal128", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			_ = CheckDecimal("12345.6789", 128, 4)
		}
	})
}
