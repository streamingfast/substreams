package db

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func statefulTestTable(t *testing.T) *TableInfo {
	t.Helper()
	return mustNewTableInfo("testschema", "state", []string{"id"}, map[string]*ColumnInfo{
		"id":   NewColumnInfo("id", "String", ""),
		"n":    NewColumnInfo("n", "Nullable(Int64)", ""),
		"u":    NewColumnInfo("u", "UInt256", ""),
		"d":    NewColumnInfo("d", "Decimal(38, 4)", ""),
		"f":    NewColumnInfo("f", "Float64", ""),
		"text": NewColumnInfo("text", "String", ""),
	})
}

func TestEvaluateOperation_Ops(t *testing.T) {
	table := statefulTestTable(t)
	pk := map[string]string{"id": "a"}
	existing := Row{"id": new("a"), "n": new("10"), "u": new("100"), "d": new("1.5000"), "f": new("0.5"), "text": new("2.50")}

	tests := []struct {
		name     string
		opType   OperationType
		current  Row
		data     map[string]FieldData
		expected Row
		outcome  rowOutcome
		err      string
	}{
		{"insert new row", OperationTypeInsert, nil, map[string]FieldData{"n": {Value: "1"}}, Row{"id": new("a"), "n": new("1")}, rowWritten, ""},
		{"insert existing row fails", OperationTypeInsert, existing, map[string]FieldData{"n": {Value: "1"}}, nil, rowUnchanged, "already exists"},

		{"upsert new row takes values as given", OperationTypeUpsert, nil,
			map[string]FieldData{"n": {Value: "5", UpdateOp: UpdateOpAdd}, "u": {Value: "7", UpdateOp: UpdateOpMax}, "d": {Value: "-3", UpdateOp: UpdateOpMin}},
			Row{"id": new("a"), "n": new("5"), "u": new("7"), "d": new("-3.0000")}, rowWritten, ""},
		{"upsert set", OperationTypeUpsert, existing, map[string]FieldData{"n": {Value: "3"}}, with(existing, "n", new("3")), rowWritten, ""},
		{"merged sum with 18 decimals into an integer column", OperationTypeUpsert, nil, map[string]FieldData{"n": {Value: "20.000000000000000000", UpdateOp: UpdateOpAdd}}, Row{"id": new("a"), "n": new("20")}, rowWritten, ""},
		{"merged sum with 18 decimals into a text column stays as is", OperationTypeUpsert, nil, map[string]FieldData{"text": {Value: "20.000000000000000000", UpdateOp: UpdateOpAdd}}, Row{"id": new("a"), "text": new("20.000000000000000000")}, rowWritten, ""},
		{"upsert add integer", OperationTypeUpsert, existing, map[string]FieldData{"n": {Value: "5", UpdateOp: UpdateOpAdd}}, with(existing, "n", new("15")), rowWritten, ""},
		{"upsert add uint256", OperationTypeUpsert, existing,
			map[string]FieldData{"u": {Value: "115792089237316195423570985008687907853269984665640564039457584007913129639835", UpdateOp: UpdateOpAdd}},
			with(existing, "u", new("115792089237316195423570985008687907853269984665640564039457584007913129639935")), rowWritten, ""},
		{"upsert add decimal keeps column scale", OperationTypeUpsert, existing, map[string]FieldData{"d": {Value: "0.25", UpdateOp: UpdateOpAdd}}, with(existing, "d", new("1.7500")), rowWritten, ""},
		{"upsert add float", OperationTypeUpsert, existing, map[string]FieldData{"f": {Value: "0.25", UpdateOp: UpdateOpAdd}}, with(existing, "f", new("0.75")), rowWritten, ""},
		{"upsert add tiny exponent to text stays exact", OperationTypeUpsert, with(existing, "text", new("0")), map[string]FieldData{"text": {Value: "1e-100", UpdateOp: UpdateOpAdd}}, with(existing, "text", new("0."+strings.Repeat("0", 99)+"1")), rowWritten, ""},
		{"upsert add text keeps operand scale", OperationTypeUpsert, existing, map[string]FieldData{"text": {Value: "1", UpdateOp: UpdateOpAdd}}, with(existing, "text", new("3.50")), rowWritten, ""},
		{"upsert add to NULL counts from 0", OperationTypeUpsert, with(existing, "n", nil), map[string]FieldData{"n": {Value: "5", UpdateOp: UpdateOpAdd}}, with(existing, "n", new("5")), rowWritten, ""},
		{"upsert max keeps larger", OperationTypeUpsert, existing, map[string]FieldData{"n": {Value: "7", UpdateOp: UpdateOpMax}}, existing, rowWritten, ""},
		{"upsert max takes larger", OperationTypeUpsert, existing, map[string]FieldData{"n": {Value: "70", UpdateOp: UpdateOpMax}}, with(existing, "n", new("70")), rowWritten, ""},
		{"upsert max of NULL and negative is 0", OperationTypeUpsert, with(existing, "n", nil), map[string]FieldData{"n": {Value: "-5", UpdateOp: UpdateOpMax}}, with(existing, "n", new("0")), rowWritten, ""},
		{"upsert min takes smaller", OperationTypeUpsert, existing, map[string]FieldData{"n": {Value: "-1", UpdateOp: UpdateOpMin}}, with(existing, "n", new("-1")), rowWritten, ""},
		{"upsert min of NULL and positive is 0", OperationTypeUpsert, with(existing, "n", nil), map[string]FieldData{"n": {Value: "5", UpdateOp: UpdateOpMin}}, with(existing, "n", new("0")), rowWritten, ""},
		{"upsert set if null keeps value", OperationTypeUpsert, existing, map[string]FieldData{"n": {Value: "1", UpdateOp: UpdateOpSetIfNull}}, existing, rowWritten, ""},
		{"upsert set if null fills NULL", OperationTypeUpsert, with(existing, "n", nil), map[string]FieldData{"n": {Value: "1", UpdateOp: UpdateOpSetIfNull}}, with(existing, "n", new("1")), rowWritten, ""},
		{"upsert add fractional to integer rounds half away from zero", OperationTypeUpsert, existing, map[string]FieldData{"n": {Value: "0.5", UpdateOp: UpdateOpAdd}}, with(existing, "n", new("11")), rowWritten, ""},
		{"upsert add fractional to negative integer rounds half away from zero", OperationTypeUpsert, with(existing, "n", new("-10")), map[string]FieldData{"n": {Value: "-0.5", UpdateOp: UpdateOpAdd}}, with(existing, "n", new("-11")), rowWritten, ""},
		{"upsert add converts its value to the column type first", OperationTypeUpsert, existing, map[string]FieldData{"n": {Value: "-0.5", UpdateOp: UpdateOpAdd}}, with(existing, "n", new("9")), rowWritten, ""},
		{"update add uses its value as given", OperationTypeUpdate, existing, map[string]FieldData{"n": {Value: "-0.5", UpdateOp: UpdateOpAdd}}, with(existing, "n", new("10")), rowWritten, ""},
		{"upsert add non-number fails", OperationTypeUpsert, existing, map[string]FieldData{"n": {Value: "abc", UpdateOp: UpdateOpAdd}}, nil, rowUnchanged, "not a number"},
		{"upsert unknown column fails", OperationTypeUpsert, existing, map[string]FieldData{"nope": {Value: "1"}}, nil, rowUnchanged, "unknown column"},

		{"update missing row does nothing", OperationTypeUpdate, nil, map[string]FieldData{"n": {Value: "1"}}, nil, rowUnchanged, ""},
		{"update add", OperationTypeUpdate, existing, map[string]FieldData{"n": {Value: "1", UpdateOp: UpdateOpAdd}}, with(existing, "n", new("11")), rowWritten, ""},

		{"delete existing row", OperationTypeDelete, existing, nil, nil, rowDeleted, ""},
		{"delete missing row does nothing", OperationTypeDelete, nil, nil, nil, rowUnchanged, ""},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			op := &Operation{table: table, opType: test.opType, primaryKey: pk, data: test.data}
			row, outcome, err := evaluateOperation(op, test.current)
			if test.err != "" {
				require.ErrorContains(t, err, test.err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, test.outcome, outcome)
			assert.Equal(t, rowValues(test.expected), rowValues(row))
		})
	}
}

func TestEvaluateOperation_DoesNotChangeCurrentRow(t *testing.T) {
	current := Row{"id": new("a"), "n": new("10")}
	op := &Operation{table: statefulTestTable(t), opType: OperationTypeUpsert, primaryKey: map[string]string{"id": "a"}, data: map[string]FieldData{"n": {Value: "1", UpdateOp: UpdateOpAdd}}}

	_, _, err := evaluateOperation(op, current)
	require.NoError(t, err)
	assert.Equal(t, "10", *current["n"])
}

func TestNumericColumnKind(t *testing.T) {
	tests := []struct {
		typ   string
		kind  numericKind
		scale int
	}{
		{"Int64", numericInteger, 0},
		{"Nullable(UInt256)", numericInteger, 0},
		{"LowCardinality(Nullable(Int8))", numericInteger, 0},
		{"Float32", numericFloat, 0},
		{"Decimal(76, 18)", numericDecimal, 18},
		{"Decimal128(6)", numericDecimal, 6},
		{"String", numericOther, 0},
		{"Interval", numericOther, 0},
	}
	for _, test := range tests {
		kind, scale := numericColumnKind(test.typ)
		assert.Equal(t, test.kind, kind, test.typ)
		assert.Equal(t, test.scale, scale, test.typ)
	}
}

func with(row Row, column string, value *string) Row {
	out := row.clone()
	out[column] = value
	return out
}

// rowValues turns a row into comparable values, NULL as "<nil>".
func rowValues(row Row) map[string]string {
	if row == nil {
		return nil
	}
	out := make(map[string]string, len(row))
	for k, v := range row {
		if v == nil {
			out[k] = "<nil>"
		} else {
			out[k] = *v
		}
	}
	return out
}
