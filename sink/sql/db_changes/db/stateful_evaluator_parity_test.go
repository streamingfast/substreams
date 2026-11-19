package db

import (
	"context"
	"database/sql"
	"maps"
	"math/big"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEvaluateOperation_MatchesPostgres runs sequences of operations through the statements
// PostgresDialect builds, against a real PostgreSQL, and through evaluateOperation, and checks
// both end with the same row. Each operation is its own flush.
func TestEvaluateOperation_MatchesPostgres(t *testing.T) {
	dsnString := os.Getenv("PG_DSN")
	if dsnString == "" {
		t.Skip(`PG_DSN not set, please specify PG_DSN to run this test, example: PG_DSN="psql://dev-node:insecure-change-me-in-prod@localhost:5432/dev-node?enable_incremental_sort=off&sslmode=disable"`)
	}

	ctx := context.Background()
	dsn, err := ParseDSN(dsnString)
	require.NoError(t, err)
	loader, err := NewLoader(dsn, testCursorTableName, testHistoryTableName, "", 0, 0, 0, OnModuleHashMismatchIgnore.String(), nil, zlog, tracer)
	require.NoError(t, err)
	t.Cleanup(func() { _ = loader.Close() })

	_, err = loader.ExecContext(ctx, `DROP TABLE IF EXISTS public.parity; CREATE TABLE public.parity (id text PRIMARY KEY, n bigint, d numeric, t text)`)
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = loader.ExecContext(ctx, `DROP TABLE IF EXISTS public.parity`) })

	// Column scan types as the loader gets them from the database, which statements depend on.
	columnTypes, err := loader.dialect.GetTableColumns(loader.DB, "public", "parity")
	require.NoError(t, err)
	pgColumns := map[string]*ColumnInfo{}
	for _, c := range columnTypes {
		pgColumns[c.Name()] = &ColumnInfo{name: c.Name(), escapedName: EscapeIdentifier(c.Name()), databaseTypeName: c.DatabaseTypeName(), scanType: c.ScanType()}
	}
	pgTable := mustNewTableInfo("public", "parity", []string{"id"}, pgColumns)
	chTable := mustNewTableInfo("default", "parity", []string{"id"}, map[string]*ColumnInfo{
		"id": NewColumnInfo("id", "String", ""),
		"n":  NewColumnInfo("n", "Nullable(Int64)", ""),
		"d":  NewColumnInfo("d", "Nullable(Decimal(38, 6))", ""),
		"t":  NewColumnInfo("t", "Nullable(String)", ""),
	})
	dialect := NewPostgresDialect("public", testCursorTableName, testHistoryTableName)

	type step struct {
		opType OperationType
		data   map[string]FieldData
	}
	set := func(v string) FieldData { return FieldData{Value: v} }
	add := func(v string) FieldData { return FieldData{Value: v, UpdateOp: UpdateOpAdd} }
	maxOp := func(v string) FieldData { return FieldData{Value: v, UpdateOp: UpdateOpMax} }
	minOp := func(v string) FieldData { return FieldData{Value: v, UpdateOp: UpdateOpMin} }
	ifNull := func(v string) FieldData { return FieldData{Value: v, UpdateOp: UpdateOpSetIfNull} }

	sequences := map[string][]step{
		"upsert deltas on a new row": {
			{OperationTypeUpsert, map[string]FieldData{"n": add("5"), "d": maxOp("-2.5"), "t": add("1.25")}},
		},
		"deltas on an existing row": {
			{OperationTypeUpsert, map[string]FieldData{"n": set("10"), "d": set("1.5"), "t": set("2.50")}},
			{OperationTypeUpsert, map[string]FieldData{"n": add("5"), "d": add("0.25"), "t": add("1")}},
			{OperationTypeUpsert, map[string]FieldData{"n": maxOp("12"), "d": minOp("-1")}},
		},
		"deltas on NULL": {
			{OperationTypeInsert, map[string]FieldData{}},
			{OperationTypeUpsert, map[string]FieldData{"n": maxOp("-5"), "d": minOp("3"), "t": add("7")}},
		},
		"fractional delta on an integer column": {
			{OperationTypeUpsert, map[string]FieldData{"n": set("10")}},
			{OperationTypeUpsert, map[string]FieldData{"n": add("0.5")}},
			{OperationTypeUpdate, map[string]FieldData{"n": add("-2.5")}},
		},
		"negative fractional upsert delta on an integer column": {
			{OperationTypeUpsert, map[string]FieldData{"n": set("10")}},
			{OperationTypeUpsert, map[string]FieldData{"n": add("-0.5")}},
			{OperationTypeUpsert, map[string]FieldData{"n": add("2.5"), "d": set("1")}},
		},
		"set if null": {
			{OperationTypeInsert, map[string]FieldData{"n": set("1")}},
			{OperationTypeUpsert, map[string]FieldData{"n": ifNull("2"), "t": ifNull("x")}},
			{OperationTypeUpsert, map[string]FieldData{"t": ifNull("y")}},
		},
		"update then delete then upsert": {
			{OperationTypeUpdate, map[string]FieldData{"n": set("1")}},
			{OperationTypeInsert, map[string]FieldData{"n": set("1")}},
			{OperationTypeUpdate, map[string]FieldData{"n": add("2")}},
			{OperationTypeDelete, nil},
			{OperationTypeUpdate, map[string]FieldData{"n": add("2")}},
			{OperationTypeUpsert, map[string]FieldData{"n": add("4")}},
		},
	}

	for name, steps := range sequences {
		t.Run(name, func(t *testing.T) {
			_, err := loader.ExecContext(ctx, `TRUNCATE public.parity`)
			require.NoError(t, err)

			pk := map[string]string{"id": "row"}
			var evaluated Row
			for _, s := range steps {
				data := maps.Clone(s.data)
				if data == nil {
					data = map[string]FieldData{}
				}
				if s.opType == OperationTypeInsert || s.opType == OperationTypeUpsert {
					data["id"] = FieldData{Value: "row"}
				}

				query, _, err := dialect.prepareStatement("public", &Operation{table: pgTable, opType: s.opType, primaryKey: pk, data: data})
				require.NoError(t, err)
				_, err = loader.ExecContext(ctx, query)
				require.NoError(t, err)

				row, outcome, err := evaluateOperation(&Operation{table: chTable, opType: s.opType, primaryKey: pk, data: data}, evaluated)
				require.NoError(t, err)
				switch outcome {
				case rowWritten:
					evaluated = row
				case rowDeleted:
					evaluated = nil
				}
			}

			stored := readParityRow(t, loader.DB)
			if stored == nil {
				assert.Nil(t, evaluated)
				return
			}
			require.NotNil(t, evaluated, "postgres has a row, evaluator has none")
			for _, column := range []string{"n", "d", "t"} {
				assertSameNumberOrNull(t, column, stored[column], evaluated[column])
			}
		})
	}
}

func readParityRow(t *testing.T, db *sql.DB) Row {
	var n, d, text sql.NullString
	err := db.QueryRow(`SELECT n::text, d::text, t FROM public.parity WHERE id = 'row'`).Scan(&n, &d, &text)
	if err == sql.ErrNoRows {
		return nil
	}
	require.NoError(t, err)

	toPtr := func(s sql.NullString) *string {
		if !s.Valid {
			return nil
		}
		return &s.String
	}
	return Row{"n": toPtr(n), "d": toPtr(d), "t": toPtr(text)}
}

// assertSameNumberOrNull compares numerically when both are numbers, as the column types differ
// between the two databases (numeric vs Decimal scale), and as text otherwise.
func assertSameNumberOrNull(t *testing.T, column string, postgres, evaluated *string) {
	t.Helper()
	if postgres == nil || evaluated == nil {
		assert.Equal(t, postgres == nil, evaluated == nil, "column %q: postgres %v, evaluator %v", column, rowValues(Row{"v": postgres}), rowValues(Row{"v": evaluated}))
		return
	}
	a, okA := new(big.Rat).SetString(*postgres)
	b, okB := new(big.Rat).SetString(*evaluated)
	if okA && okB {
		assert.Zero(t, a.Cmp(b), "column %q: postgres %s, evaluator %s", column, *postgres, *evaluated)
		return
	}
	assert.Equal(t, *postgres, *evaluated, "column %q", column)
}
