package db

import (
	"context"
	"fmt"
	"math/big"
	"math/rand/v2"
	"os"
	"strings"
	"testing"

	"github.com/bobg/go-generics/v2/slices"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEscapeColumns(t *testing.T) {
	ctx := context.Background()
	dsnString := os.Getenv("PG_DSN")
	if dsnString == "" {
		t.Skip(`PG_DSN not set, please specify PG_DSN to run this test, example: PG_DSN="psql://dev-node:insecure-change-me-in-prod@localhost:5432/dev-node?enable_incremental_sort=off&sslmode=disable"`)
	}

	dsn, err := ParseDSN(dsnString)
	require.NoError(t, err)

	dbLoader, err := NewLoader(
		dsn,
		testCursorTableName,
		testHistoryTableName,
		"cluster.name.1",
		0, 0, 0,
		OnModuleHashMismatchIgnore.String(),
		nil,
		zlog, tracer,
	)
	require.NoError(t, err)

	tx, err := dbLoader.DB.Begin()
	require.NoError(t, err)

	colInputs := []string{
		"regular",

		"from", // reserved keyword

		"withnewline\nafter",
		"withtab\tafter",
		"withreturn\rafter",
		"withbackspace\bafter",
		"withformfeed\fafter",

		`withdoubleQuote"aftersdf`,
		`withbackslash\after`,
		`withsinglequote'after`,
	}

	columnDefs := strings.Join(slices.Map(colInputs, func(str string) string {
		return fmt.Sprintf("%s text", EscapeIdentifier(str))
	}), ",")

	createStatement := fmt.Sprintf(`create table "test" (%s)`, columnDefs)
	_, err = tx.ExecContext(ctx, createStatement)
	require.NoError(t, err)

	columns := strings.Join(slices.Map(colInputs, EscapeIdentifier), ",")
	values := strings.Join(slices.Map(colInputs, func(str string) string { return `'any'` }), ",")
	insertStatement := fmt.Sprintf(`insert into "test" (%s) values (%s)`, columns, values)

	_, err = tx.ExecContext(ctx, insertStatement)
	require.NoError(t, err)

	err = tx.Rollback()
	require.NoError(t, err)
}

func TestEscapeValues(t *testing.T) {

	ctx := context.Background()
	dsnString := os.Getenv("PG_DSN")
	if dsnString == "" {
		t.Skip(`PG_DSN not set, please specify PG_DSN to run this test, example: PG_DSN="psql://dev-node:insecure-change-me-in-prod@localhost:5432/dev-node?enable_incremental_sort=off&sslmode=disable"`)
	}

	dsn, err := ParseDSN(dsnString)
	require.NoError(t, err)

	dbLoader, err := NewLoader(
		dsn,
		testCursorTableName,
		testHistoryTableName,
		"cluster.name.1",
		0, 0, 0,
		OnModuleHashMismatchIgnore.String(),
		nil,
		zlog, tracer,
	)
	require.NoError(t, err)

	tx, err := dbLoader.DB.Begin()
	require.NoError(t, err)

	createStatement := `create table "test" ("col" text);`
	_, err = tx.ExecContext(ctx, createStatement)
	require.NoError(t, err)

	err = tx.Commit()
	require.NoError(t, err)

	defer func() {
		_, err = dbLoader.DB.ExecContext(ctx, `drop table "test"`)
		require.NoError(t, err)
	}()

	valueStrings := []string{
		`regularValue`,

		`withApostrophe'`,

		"withNewlineCharNone\nafter",
		"withTabCharNone\tafter",
		"withCarriageReturnCharNone\rafter",
		"withBackspaceCharNone\bafter",
		"withFormFeedCharNone\fafter",

		`with\nNewlineLiteral`,

		`with'singleQuote`,
		`withDoubleQuote"`,
		`withSingle\Backslash`,

		`withExoticCharacterNone中文`,
	}

	for _, str := range valueStrings {
		t.Run(str, func(tt *testing.T) {

			tx, err := dbLoader.DB.Begin()
			require.NoError(t, err)

			insertStatement := fmt.Sprintf(`insert into "test" ("col") values (%s);`, escapeStringValue(str))
			_, err = tx.ExecContext(ctx, insertStatement)
			require.NoError(tt, err)

			checkStatement := `select "col" from "test";`
			row := tx.QueryRowContext(ctx, checkStatement)
			var value string
			err = row.Scan(&value)
			require.NoError(tt, err)
			require.Equal(tt, str, value, "Inserted value is not equal to the expected value")

			err = tx.Rollback()
			require.NoError(tt, err)
		})
	}
}

func Test_prepareColValues(t *testing.T) {
	type args struct {
		table     *TableInfo
		colValues map[string]FieldData
	}
	tests := []struct {
		name        string
		args        args
		wantColumns []string
		wantValues  []string
		assertion   require.ErrorAssertionFunc
	}{
		{
			"bool true",
			args{
				newTable(t, "schemaName", "name", "id", NewColumnInfo("col", "bool", true)),
				map[string]FieldData{"col": {Value: "true", UpdateOp: UpdateOpSet}},
			},
			[]string{`"col"`},
			[]string{`'true'`},
			require.NoError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dialect := PostgresDialect{}

			gotColumns, gotValues, _, _, err := dialect.prepareColValues(tt.args.table, tt.args.colValues)
			tt.assertion(t, err)
			assert.Equal(t, tt.wantColumns, gotColumns)
			assert.Equal(t, tt.wantValues, gotValues)
		})
	}
}

func newTable(t *testing.T, schema, name, primaryColumn string, columnInfos ...*ColumnInfo) *TableInfo {
	columns := make(map[string]*ColumnInfo)
	columns[primaryColumn] = NewColumnInfo(primaryColumn, "text", "")
	for _, columnInfo := range columnInfos {
		columns[columnInfo.name] = columnInfo
	}

	table, err := NewTableInfo("public", "data", []string{"id"}, columns)
	require.NoError(t, err)

	return table
}

// TestMergeData_ValidTransitions tests all valid UpdateOp transitions
func TestMergeData_ValidTransitions(t *testing.T) {
	tests := []struct {
		name          string
		existingOp    UpdateOp
		existingValue string
		incomingOp    UpdateOp
		incomingValue string
		expectedValue string
		expectedOp    UpdateOp
	}{
		// SET → any (all allowed)
		{"SET → SET", UpdateOpSet, "100", UpdateOpSet, "200", "200", UpdateOpSet},
		{"SET → ADD", UpdateOpSet, "100", UpdateOpAdd, "50", "150", UpdateOpSet},
		{"SET → MAX", UpdateOpSet, "100", UpdateOpMax, "150", "150", UpdateOpSet},
		{"SET → MAX (existing wins)", UpdateOpSet, "200", UpdateOpMax, "150", "200", UpdateOpSet},
		{"SET → MIN", UpdateOpSet, "100", UpdateOpMin, "50", "50", UpdateOpSet},
		{"SET → MIN (existing wins)", UpdateOpSet, "50", UpdateOpMin, "100", "50", UpdateOpSet},
		{"SET → SET_IF_NULL", UpdateOpSet, "100", UpdateOpSetIfNull, "200", "100", UpdateOpSet},

		// ADD → ADD (accumulates)
		{"ADD → ADD", UpdateOpAdd, "100", UpdateOpAdd, "50", "150", UpdateOpAdd},
		{"ADD → ADD (negative)", UpdateOpAdd, "100", UpdateOpAdd, "-30", "70", UpdateOpAdd},

		// MAX → MAX (computes max)
		{"MAX → MAX (new wins)", UpdateOpMax, "100", UpdateOpMax, "150", "150", UpdateOpMax},
		{"MAX → MAX (existing wins)", UpdateOpMax, "200", UpdateOpMax, "150", "200", UpdateOpMax},

		// MIN → MIN (computes min)
		{"MIN → MIN (new wins)", UpdateOpMin, "100", UpdateOpMin, "50", "50", UpdateOpMin},
		{"MIN → MIN (existing wins)", UpdateOpMin, "50", UpdateOpMin, "100", "50", UpdateOpMin},

		// SET_IF_NULL → SET_IF_NULL (first value wins)
		{"SET_IF_NULL → SET_IF_NULL", UpdateOpSetIfNull, "100", UpdateOpSetIfNull, "200", "100", UpdateOpSetIfNull},

		// any → SET (SET always overwrites)
		{"ADD → SET", UpdateOpAdd, "100", UpdateOpSet, "200", "200", UpdateOpSet},
		{"MAX → SET", UpdateOpMax, "100", UpdateOpSet, "200", "200", UpdateOpSet},
		{"MIN → SET", UpdateOpMin, "100", UpdateOpSet, "200", "200", UpdateOpSet},
		{"SET_IF_NULL → SET", UpdateOpSetIfNull, "100", UpdateOpSet, "200", "200", UpdateOpSet},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			op := &Operation{
				opType: OperationTypeUpsert,
				data:   map[string]FieldData{"field": {Value: tt.existingValue, UpdateOp: tt.existingOp}},
			}

			err := op.mergeData(map[string]FieldData{"field": {Value: tt.incomingValue, UpdateOp: tt.incomingOp}})
			require.NoError(t, err)

			assert.Equal(t, tt.expectedValue, op.data["field"].Value)
			assert.Equal(t, tt.expectedOp, op.data["field"].UpdateOp)
		})
	}
}

// TestMergeData_InvalidTransitions tests all invalid UpdateOp transitions return errors
func TestMergeData_InvalidTransitions(t *testing.T) {
	tests := []struct {
		name       string
		existingOp UpdateOp
		incomingOp UpdateOp
	}{
		// ADD → others (except ADD and SET)
		{"ADD → MAX", UpdateOpAdd, UpdateOpMax},
		{"ADD → MIN", UpdateOpAdd, UpdateOpMin},
		{"ADD → SET_IF_NULL", UpdateOpAdd, UpdateOpSetIfNull},

		// MAX → others (except MAX and SET)
		{"MAX → ADD", UpdateOpMax, UpdateOpAdd},
		{"MAX → MIN", UpdateOpMax, UpdateOpMin},
		{"MAX → SET_IF_NULL", UpdateOpMax, UpdateOpSetIfNull},

		// MIN → others (except MIN and SET)
		{"MIN → ADD", UpdateOpMin, UpdateOpAdd},
		{"MIN → MAX", UpdateOpMin, UpdateOpMax},
		{"MIN → SET_IF_NULL", UpdateOpMin, UpdateOpSetIfNull},

		// SET_IF_NULL → others (except SET_IF_NULL and SET)
		{"SET_IF_NULL → ADD", UpdateOpSetIfNull, UpdateOpAdd},
		{"SET_IF_NULL → MAX", UpdateOpSetIfNull, UpdateOpMax},
		{"SET_IF_NULL → MIN", UpdateOpSetIfNull, UpdateOpMin},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			op := &Operation{
				opType: OperationTypeUpsert,
				data:   map[string]FieldData{"field": {Value: "100", UpdateOp: tt.existingOp}},
			}

			err := op.mergeData(map[string]FieldData{"field": {Value: "200", UpdateOp: tt.incomingOp}})
			require.Error(t, err)
			assert.Contains(t, err.Error(), "invalid UpdateOp transition")
		})
	}
}

// TestMergeData_NewField tests adding a new field (no existing)
func TestMergeData_NewField(t *testing.T) {
	ops := []UpdateOp{UpdateOpSet, UpdateOpAdd, UpdateOpMax, UpdateOpMin, UpdateOpSetIfNull}

	for _, op := range ops {
		t.Run(updateOpName(op), func(t *testing.T) {
			operation := &Operation{
				opType: OperationTypeUpsert,
				data:   map[string]FieldData{},
			}

			err := operation.mergeData(map[string]FieldData{"field": {Value: "100", UpdateOp: op}})
			require.NoError(t, err)
			assert.Equal(t, "100", operation.data["field"].Value)
			assert.Equal(t, op, operation.data["field"].UpdateOp)
		})
	}
}

// TestMergeData_NonNumeric tests handling of non-numeric values
func TestMergeData_NonNumeric(t *testing.T) {
	tests := []struct {
		name          string
		existingValue string
		incomingValue string
		op            UpdateOp
		expectedValue string
	}{
		{"ADD non-numeric", "hello", "world", UpdateOpAdd, "world"},
		{"MAX non-numeric", "hello", "world", UpdateOpMax, "world"},
		{"MIN non-numeric", "hello", "world", UpdateOpMin, "world"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			op := &Operation{
				opType: OperationTypeUpsert,
				data:   map[string]FieldData{"field": {Value: tt.existingValue, UpdateOp: tt.op}},
			}

			err := op.mergeData(map[string]FieldData{"field": {Value: tt.incomingValue, UpdateOp: tt.op}})
			require.NoError(t, err)
			assert.Equal(t, tt.expectedValue, op.data["field"].Value)
		})
	}
}

// TestMergeData_DecimalPrecision tests high precision decimal handling
func TestMergeData_DecimalPrecision(t *testing.T) {
	tests := []struct {
		name          string
		existingValue string
		incomingValue string
		expectedValue string
	}{
		{"small decimals", "0.000000000000000001", "0.000000000000000002", "0.000000000000000003"},
		{"large numbers", "1000000000000000000", "1000000000000000000", "2000000000000000000"},
		{"uint256", "115792089237316195423570985008687907853269984665640564039457584007913129639935", "-1", "115792089237316195423570985008687907853269984665640564039457584007913129639934"},
		{"more than 18 decimals", "0.0000000000000000000001", "0.0000000000000000000002", "0.0000000000000000000003"},
		{"keeps the larger scale", "1.50", "2", "3.50"},
		{"exponent", "1.5e3", "2.5e-2", "1500.025"},
		{"mixed precision", "100.5", "50.25", "150.75"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			op := &Operation{
				opType: OperationTypeUpsert,
				data:   map[string]FieldData{"field": {Value: tt.existingValue, UpdateOp: UpdateOpAdd}},
			}

			err := op.mergeData(map[string]FieldData{"field": {Value: tt.incomingValue, UpdateOp: UpdateOpAdd}})
			require.NoError(t, err)
			assert.Equal(t, tt.expectedValue, op.data["field"].Value)
		})
	}
}

// TestMergeData_MultipleFields tests merging multiple fields at once
func TestMergeData_MultipleFields(t *testing.T) {
	op := &Operation{
		opType: OperationTypeUpsert,
		data: map[string]FieldData{
			"counter":    {Value: "100", UpdateOp: UpdateOpAdd},
			"max_price":  {Value: "50", UpdateOp: UpdateOpMax},
			"min_price":  {Value: "100", UpdateOp: UpdateOpMin},
			"first_seen": {Value: "2024-01-01", UpdateOp: UpdateOpSetIfNull},
			"name":       {Value: "old", UpdateOp: UpdateOpSet},
		},
	}

	incoming := map[string]FieldData{
		"counter":    {Value: "50", UpdateOp: UpdateOpAdd},
		"max_price":  {Value: "75", UpdateOp: UpdateOpMax},
		"min_price":  {Value: "80", UpdateOp: UpdateOpMin},
		"first_seen": {Value: "2024-02-01", UpdateOp: UpdateOpSetIfNull},
		"name":       {Value: "new", UpdateOp: UpdateOpSet},
	}

	err := op.mergeData(incoming)
	require.NoError(t, err)

	assert.Equal(t, "150", op.data["counter"].Value)
	assert.Equal(t, UpdateOpAdd, op.data["counter"].UpdateOp)

	assert.Equal(t, "75", op.data["max_price"].Value)
	assert.Equal(t, UpdateOpMax, op.data["max_price"].UpdateOp)

	assert.Equal(t, "80", op.data["min_price"].Value)
	assert.Equal(t, UpdateOpMin, op.data["min_price"].UpdateOp)

	assert.Equal(t, "2024-01-01", op.data["first_seen"].Value)
	assert.Equal(t, UpdateOpSetIfNull, op.data["first_seen"].UpdateOp)

	assert.Equal(t, "new", op.data["name"].Value)
	assert.Equal(t, UpdateOpSet, op.data["name"].UpdateOp)
}

// TestMergeData_DeleteOperation tests that merging into a delete operation fails
func TestMergeData_DeleteOperation(t *testing.T) {
	op := &Operation{
		opType: OperationTypeDelete,
		data:   map[string]FieldData{},
	}

	err := op.mergeData(map[string]FieldData{"field": {Value: "100", UpdateOp: UpdateOpSet}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "delete operation")
}


// TestMergeData_SameAsAppliedOneByOne checks merged sums are exact: what PostgreSQL's numeric gives
// adding the values one by one, however the changes were batched.
func TestMergeData_SameAsAppliedOneByOne(t *testing.T) {
	r := rand.New(rand.NewPCG(1, 2))
	number := func() string {
		switch r.IntN(5) {
		case 0:
			return fmt.Sprint(r.Int64N(1_000_000) - 500_000)
		case 1:
			return new(big.Int).Lsh(big.NewInt(r.Int64()), uint(r.IntN(190))).String()
		case 2:
			return fmt.Sprintf("%d.%0*d", r.Int64N(1000)-500, r.IntN(30)+1, r.Int64N(1_000_000_000))
		case 3:
			return fmt.Sprintf("-0.%040d", r.Int64N(1_000_000_000_000))
		default:
			return fmt.Sprintf("%de%d", r.Int64N(1000), r.IntN(20)-10)
		}
	}
	exact := func(values []string) *big.Rat {
		sum := new(big.Rat)
		for _, v := range values {
			x, ok := new(big.Rat).SetString(v)
			require.True(t, ok, v)
			sum.Add(sum, x)
		}
		return sum
	}

	for i := 0; i < 5000; i++ {
		values := []string{number(), number(), number(), number()}
		op := &Operation{opType: OperationTypeUpsert, data: map[string]FieldData{"v": {Value: values[0], UpdateOp: UpdateOpAdd}}}
		for _, v := range values[1:] {
			require.NoError(t, op.mergeData(map[string]FieldData{"v": {Value: v, UpdateOp: UpdateOpAdd}}))
		}
		merged, ok := new(big.Rat).SetString(op.data["v"].Value)
		require.True(t, ok, op.data["v"].Value)
		require.Zero(t, merged.Cmp(exact(values)), "%v gave %s", values, op.data["v"].Value)
	}
}

func TestMergeData_HugeExponentIsNotANumber(t *testing.T) {
	op := &Operation{opType: OperationTypeUpsert, data: map[string]FieldData{"v": {Value: "1", UpdateOp: UpdateOpAdd}}}
	require.NoError(t, op.mergeData(map[string]FieldData{"v": {Value: "1e99999999", UpdateOp: UpdateOpAdd}}))
	assert.Equal(t, "1e99999999", op.data["v"].Value, "not numeric: the latest value wins, as for any non-numeric value")
}
