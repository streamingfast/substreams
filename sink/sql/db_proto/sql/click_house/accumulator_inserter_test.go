package clickhouse

import (
	"testing"

	"github.com/ClickHouse/ch-go/proto"
	"github.com/streamingfast/logging"
	v1 "github.com/streamingfast/substreams/pb/sf/substreams/sink/sql/schema/v1"
	"github.com/streamingfast/substreams/sink/sql/db_proto/sql/schema"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

var _, testTracer = logging.PackageLogger("clickhouse_test", "github.com/streamingfast/substreams/sink/sql/db_proto/sql/click_house/tests")

func numericStringInserter() (*AccumulatorInserter, *proto.ColUInt256, *proto.ColInt128, *ColScaledDecimal128) {
	uint256Col := &proto.ColUInt256{}
	int128Col := &proto.ColInt128{}
	decimalCol := &ColScaledDecimal128{ColDecimal128: &proto.ColDecimal128{}, scale: 2}

	inserter := &AccumulatorInserter{
		accumulators: map[string]*accumulator{"t": {
			tableName: "t",
			columns: map[int]*schema.Column{
				0: {Name: "amount"},
				1: {Name: "delta"},
				2: {Name: "price", ConvertTo: &v1.StringConvertion{Convertion: &v1.StringConvertion_Decimal128{Decimal128: &v1.Decimal128{Scale: 2}}}},
			},
			input: map[string]proto.ColInput{"amount": uint256Col, "delta": int128Col, "price": decimalCol},
		}},
		logger: zap.NewNop(),
		tracer: testTracer,
	}

	return inserter, uint256Col, int128Col, decimalCol
}

func TestInsertStoresAnEmptyNumericStringAsZero(t *testing.T) {
	inserter, uint256Col, int128Col, decimalCol := numericStringInserter()

	require.NoError(t, inserter.insert("t", []any{"", "", ""}))
	require.Equal(t, proto.UInt256{}, uint256Col.Row(0))
	require.Equal(t, proto.Int128{}, int128Col.Row(0))
	require.Equal(t, proto.Decimal128{}, decimalCol.Row(0))
}

func TestInsertReturnsAnErrorForAnInvalidNumericString(t *testing.T) {
	cases := []struct {
		name string
		row  []any
	}{
		{name: "not a number", row: []any{"abc", "1", "1"}},
		{name: "out of range", row: []any{"1", "170141183460469231731687303715884105728", "1"}},
		{name: "bad decimal", row: []any{"1", "1", "1.2.3"}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			inserter, _, _, _ := numericStringInserter()
			require.NotPanics(t, func() {
				require.Error(t, inserter.insert("t", c.row))
			})
		})
	}
}
