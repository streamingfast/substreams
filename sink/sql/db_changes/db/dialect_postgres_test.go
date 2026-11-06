package db

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrimaryKeyToJSON(t *testing.T) {

	tests := []struct {
		name   string
		keys   map[string]string
		expect string
	}{
		{
			name: "single key",
			keys: map[string]string{
				"id": "0xdeadbeef",
			},
			expect: `{"id":"0xdeadbeef"}`,
		},
		{
			name: "two keys",
			keys: map[string]string{
				"hash": "0xdeadbeef",
				"idx":  "5",
			},
			expect: `{"hash":"0xdeadbeef","idx":"5"}`,
		},
		{
			name: "determinism",
			keys: map[string]string{
				"bbb": "1",
				"ccc": "2",
				"aaa": "3",
				"ddd": "4",
			},
			expect: `{"aaa":"3","bbb":"1","ccc":"2","ddd":"4"}`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			jsonKey := primaryKeyToJSON(test.keys)
			assert.Equal(t, test.expect, jsonKey)
		})
	}

}

func TestJSONToPrimaryKey(t *testing.T) {

	tests := []struct {
		name   string
		in     string
		expect map[string]string
	}{
		{
			name: "single key",
			in:   `{"id":"0xdeadbeef"}`,
			expect: map[string]string{
				"id": "0xdeadbeef",
			},
		},
		{
			name: "two keys",
			in:   `{"hash":"0xdeadbeef","idx":"5"}`,
			expect: map[string]string{
				"hash": "0xdeadbeef",
				"idx":  "5",
			},
		},
		{
			name: "determinism",
			in:   `{"aaa":"3","bbb":"1","ccc":"2","ddd":"4"}`,
			expect: map[string]string{
				"bbb": "1",
				"ccc": "2",
				"aaa": "3",
				"ddd": "4",
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			out, err := jsonToPrimaryKey(test.in)
			require.NoError(t, err)
			assert.Equal(t, test.expect, out)
		})
	}

}

func TestGetPrimaryKeyFakeEmptyValues(t *testing.T) {
	tests := []struct {
		name       string
		primaryKey map[string]string
		expected   string
	}{
		{
			name: "single key",
			primaryKey: map[string]string{
				"id": "value-not-used",
			},
			expected: `'' "id"`,
		},
		{
			name: "multiple keys",
			primaryKey: map[string]string{
				"id":    "value-not-used",
				"block": "value-not-used",
				"idx":   "value-not-used",
			},
			expected: `'' "block",'' "id",'' "idx"`,
		},
		{
			name: "keys with special characters",
			primaryKey: map[string]string{
				"user_id":   "value-not-used",
				"order-num": "value-not-used",
			},
			expected: `'' "order-num",'' "user_id"`,
		},
		{
			name:       "empty map",
			primaryKey: map[string]string{},
			expected:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getPrimaryKeyFakeEmptyValues(tt.primaryKey)
			assert.Equal(t, tt.expected, result)

			// For multiple keys, verify the order is predictable (alphabetical)
			if len(tt.primaryKey) > 1 {
				parts := strings.Split(result, ",")
				for i := 1; i < len(parts); i++ {
					assert.True(t, strings.Compare(parts[i-1], parts[i]) <= 0,
						"Expected sorted keys, but got %s before %s", parts[i-1], parts[i])
				}
			}
		})
	}
}

func TestGetPrimaryKeyFakeEmptyValuesAssertion(t *testing.T) {
	tests := []struct {
		name             string
		primaryKey       map[string]string
		escapedTableName string
		expected         string
	}{
		{
			name: "single key",
			primaryKey: map[string]string{
				"id": "value-not-used",
			},
			escapedTableName: `"users"`,
			expected:         `"users"."id" IS NULL`,
		},
		{
			name: "multiple keys",
			primaryKey: map[string]string{
				"id":    "value-not-used",
				"block": "value-not-used",
				"idx":   "value-not-used",
			},
			escapedTableName: `"transactions"`,
			expected:         `"transactions"."block" IS NULL AND "transactions"."id" IS NULL AND "transactions"."idx" IS NULL`,
		},
		{
			name: "schema qualified table",
			primaryKey: map[string]string{
				"user_id": "value-not-used",
			},
			escapedTableName: `"public"."users"`,
			expected:         `"public"."users"."user_id" IS NULL`,
		},
		{
			name:             "empty map",
			primaryKey:       map[string]string{},
			escapedTableName: `"table"`,
			expected:         "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getPrimaryKeyFakeEmptyValuesAssertion(tt.primaryKey, tt.escapedTableName)
			assert.Equal(t, tt.expected, result)

			// For multiple keys, verify the order is predictable (alphabetical)
			if len(tt.primaryKey) > 1 {
				parts := strings.Split(result, "AND ")
				for i := 1; i < len(parts); i++ {
					assert.True(t, strings.Compare(parts[i-1], parts[i]) <= 0,
						"Expected sorted parts, but got %s before %s", parts[i-1], parts[i])
				}
			}
		})
	}
}

func TestRevertOp(t *testing.T) {

	type row struct {
		op         string
		table_name string
		pk         string
		prev_value string
	}

	tests := []struct {
		name   string
		row    row
		expect string
	}{
		{
			name: "rollback insert row",
			row: row{
				op:         "I",
				table_name: `"testschema"."xfer"`,
				pk:         `{"id":"2345"}`,
				prev_value: "", // unused
			},
			expect: `DELETE FROM "testschema"."xfer" WHERE "id" = '2345';`,
		},
		{
			name: "rollback delete row",
			row: row{
				op:         "D",
				table_name: `"testschema"."xfer"`,
				pk:         `{"id":"2345"}`,
				prev_value: `{"id":"2345","sender":"0xdead","receiver":"0xbeef"}`,
			},
			expect: `INSERT INTO "testschema"."xfer" SELECT * FROM json_populate_record(null::"testschema"."xfer",` +
				`'{"id":"2345","sender":"0xdead","receiver":"0xbeef"}');`,
		},
		{
			name: "rollback update row",
			row: row{
				op:         "U",
				table_name: `"testschema"."xfer"`,
				pk:         `{"id":"2345"}`,
				prev_value: `{"id":"2345","sender":"0xdead","receiver":"0xbeef"}`,
			},
			expect: `UPDATE "testschema"."xfer" SET("id","receiver","sender")=((SELECT "id","receiver","sender" FROM json_populate_record(null::"testschema"."xfer",` +
				`'{"id":"2345","sender":"0xdead","receiver":"0xbeef"}'))) WHERE "id" = '2345';`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tx := &TestTx{}
			ctx := context.Background()
			pd := PostgresDialect{}

			row := test.row
			err := pd.revertOp(tx, ctx, row.op, row.table_name, row.pk, row.prev_value, 9999)
			require.NoError(t, err)
			assert.Equal(t, []string{test.expect}, tx.Results())
		})
	}

}

// TestPrepareStatement_UpdateOp tests SQL generation for each UpdateOp type
func TestPrepareStatement_UpdateOp(t *testing.T) {
	// Create a test table with numeric column
	table := createTestTable(t, "test_table", "id", "amount")

	tests := []struct {
		name      string
		opType    OperationType
		updateOp  UpdateOp
		value     string
		expectSQL string // substring to check in generated SQL
	}{
		// UPSERT with different UpdateOps
		{
			name:      "UPSERT SET",
			opType:    OperationTypeUpsert,
			updateOp:  UpdateOpSet,
			value:     "100",
			expectSQL: `"amount"=EXCLUDED."amount"`,
		},
		{
			name:      "UPSERT ADD",
			opType:    OperationTypeUpsert,
			updateOp:  UpdateOpAdd,
			value:     "100",
			expectSQL: `"amount"=(COALESCE("test_table"."amount"::numeric, 0) + EXCLUDED."amount"::numeric)`,
		},
		{
			name:      "UPSERT MAX",
			opType:    OperationTypeUpsert,
			updateOp:  UpdateOpMax,
			value:     "100",
			expectSQL: `"amount"=GREATEST(COALESCE("test_table"."amount"::numeric, 0), EXCLUDED."amount"::numeric)`,
		},
		{
			name:      "UPSERT MIN",
			opType:    OperationTypeUpsert,
			updateOp:  UpdateOpMin,
			value:     "100",
			expectSQL: `"amount"=LEAST(COALESCE("test_table"."amount"::numeric, 0), EXCLUDED."amount"::numeric)`,
		},
		{
			name:      "UPSERT SET_IF_NULL",
			opType:    OperationTypeUpsert,
			updateOp:  UpdateOpSetIfNull,
			value:     "100",
			expectSQL: `"amount"=COALESCE("test_table"."amount", EXCLUDED."amount")`,
		},

		// UPDATE with different UpdateOps
		{
			name:      "UPDATE SET",
			opType:    OperationTypeUpdate,
			updateOp:  UpdateOpSet,
			value:     "100",
			expectSQL: `"amount"=100`,
		},
		{
			name:      "UPDATE ADD",
			opType:    OperationTypeUpdate,
			updateOp:  UpdateOpAdd,
			value:     "100",
			expectSQL: `"amount"=(COALESCE("amount"::numeric, 0) + 100::numeric)`,
		},
		{
			name:      "UPDATE MAX",
			opType:    OperationTypeUpdate,
			updateOp:  UpdateOpMax,
			value:     "100",
			expectSQL: `"amount"=GREATEST(COALESCE("amount"::numeric, 0), 100::numeric)`,
		},
		{
			name:      "UPDATE MIN",
			opType:    OperationTypeUpdate,
			updateOp:  UpdateOpMin,
			value:     "100",
			expectSQL: `"amount"=LEAST(COALESCE("amount"::numeric, 0), 100::numeric)`,
		},
		{
			name:      "UPDATE SET_IF_NULL",
			opType:    OperationTypeUpdate,
			updateOp:  UpdateOpSetIfNull,
			value:     "100",
			expectSQL: `"amount"=COALESCE("amount", 100)`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dialect := PostgresDialect{schemaName: "public"}
			op := &Operation{
				table:      table,
				opType:     tt.opType,
				primaryKey: map[string]string{"id": "123"},
				data: map[string]FieldData{
					"amount": {Value: tt.value, UpdateOp: tt.updateOp},
				},
			}

			sql, _, err := dialect.prepareStatement("public", op)
			require.NoError(t, err)
			assert.Contains(t, sql, tt.expectSQL, "SQL should contain expected UpdateOp clause")
		})
	}
}

// TestPrepareStatement_INSERT_IgnoresUpdateOp tests that INSERT ignores UpdateOp (always direct values)
func TestPrepareStatement_INSERT_IgnoresUpdateOp(t *testing.T) {
	table := createTestTable(t, "test_table", "id", "amount")

	// For INSERT, UpdateOp should not affect the SQL - it's always a direct INSERT
	ops := []UpdateOp{UpdateOpSet, UpdateOpAdd, UpdateOpMax, UpdateOpMin, UpdateOpSetIfNull}

	for _, updateOp := range ops {
		t.Run(updateOpName(updateOp), func(t *testing.T) {
			dialect := PostgresDialect{schemaName: "public"}
			op := &Operation{
				table:      table,
				opType:     OperationTypeInsert,
				primaryKey: map[string]string{"id": "123"},
				data: map[string]FieldData{
					"amount": {Value: "100", UpdateOp: updateOp},
				},
			}

			sql, _, err := dialect.prepareStatement("public", op)
			require.NoError(t, err)
			// INSERT should always be a simple INSERT regardless of UpdateOp
			assert.Contains(t, sql, "INSERT INTO")
			assert.Contains(t, sql, "VALUES")
			assert.NotContains(t, sql, "ON CONFLICT", "INSERT should not have ON CONFLICT clause")
		})
	}
}

// createTestTable creates a TableInfo for testing with numeric columns
func createTestTable(t *testing.T, name, pkCol string, extraCols ...string) *TableInfo {
	t.Helper()
	columns := make(map[string]*ColumnInfo)

	// Primary key column (text)
	columns[pkCol] = NewColumnInfo(pkCol, "text", "")

	// Extra columns (numeric for UpdateOp testing)
	for _, col := range extraCols {
		columns[col] = NewColumnInfo(col, "numeric", int64(0))
	}

	table, err := NewTableInfo("public", name, []string{pkCol}, columns)
	require.NoError(t, err)
	return table
}
