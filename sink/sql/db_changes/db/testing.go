package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"

	"maps"

	"github.com/streamingfast/logging"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

const testCursorTableName = "cursors"
const testHistoryTableName = "substreams_history"

func NewTestLoader(
	t *testing.T,
	dsnRaw string,
	testTx *TestTx,
	tables map[string]*TableInfo,
	zlog *zap.Logger,
	tracer logging.Tracer,
) *Loader {
	dsn, err := ParseDSN(dsnRaw)
	require.NoError(t, err)

	loader, err := NewLoader(
		dsn,
		testCursorTableName,
		testHistoryTableName,
		"",
		0, 0, 0,
		OnModuleHashMismatchIgnore.String(),
		nil,
		zlog, tracer,
	)
	require.NoError(t, err)

	if testTx != nil {
		loader.testTx = testTx
	}
	loader.tables = tables
	loader.cursorTable = tables[testCursorTableName]
	return loader

}

func TestSinglePrimaryKeyTables(schema string) map[string]*TableInfo {
	return TestTables(schema, map[string]*TableInfo{
		"xfer": mustNewTableInfo(schema, "xfer", []string{"id"}, map[string]*ColumnInfo{
			"id":   NewColumnInfo("id", "text", ""),
			"from": NewColumnInfo("from", "text", ""),
			"to":   NewColumnInfo("to", "text", ""),
		}),
	})
}

func TestTables(schema string, customTable map[string]*TableInfo) map[string]*TableInfo {
	out := map[string]*TableInfo{}

	addCursorsTable(schema, out)
	maps.Copy(out, customTable)

	return out
}

func addCursorsTable(schema string, into map[string]*TableInfo) {
	into[testCursorTableName] = mustNewTableInfo(schema, testCursorTableName, []string{"id"}, map[string]*ColumnInfo{
		"block_num": NewColumnInfo("block_num", "bigint", ""),
		"block_id":  NewColumnInfo("block_id", "text", ""),
		"cursor":    NewColumnInfo("cursor", "text", ""),
		"id":        NewColumnInfo("id", "text", ""),
	})
}

func GenerateCreateTableSQL(tables map[string]*TableInfo) string {
	var sqlStatements []string
	for _, tableInfo := range tables {
		if tableInfo.name == testCursorTableName {
			continue
		}

		var columns []string
		for _, colInfo := range tableInfo.columnsByName {
			columns = append(columns, fmt.Sprintf("%s %s", colInfo.escapedName, colInfo.databaseTypeName))
		}
		var pkColumns []string
		for _, pkCol := range tableInfo.primaryColumns {
			pkColumns = append(pkColumns, pkCol.escapedName)
		}
		pk := fmt.Sprintf("PRIMARY KEY (%s)", strings.Join(pkColumns, ", "))
		columns = append(columns, pk)
		createStmt := fmt.Sprintf(
			"CREATE TABLE %s (%s);",
			tableInfo.identifier,
			strings.Join(columns, ", "),
		)
		sqlStatements = append(sqlStatements, createStmt)
	}
	return strings.Join(sqlStatements, "\n")
}

func mustNewTableInfo(schema, name string, pkList []string, columnsByName map[string]*ColumnInfo) *TableInfo {
	ti, err := NewTableInfo(schema, name, pkList, columnsByName)
	if err != nil {
		panic(err)
	}
	return ti
}

type TestTx struct {
	queries []string
	next    []*sql.Rows
}

func (t *TestTx) Rollback() error {
	t.queries = append(t.queries, "ROLLBACK")
	return nil
}

func (t *TestTx) Commit() error {
	t.queries = append(t.queries, "COMMIT")
	return nil
}

func (t *TestTx) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	t.queries = append(t.queries, query)
	return &testResult{}, nil
}

func (t *TestTx) Results() []string {
	return t.queries
}

func (t *TestTx) QueryContext(ctx context.Context, query string, args ...any) (out *sql.Rows, err error) {
	t.queries = append(t.queries, query)
	return nil, nil
}

type testResult struct{}

func (t *testResult) LastInsertId() (int64, error) {
	return 0, nil
}

func (t *testResult) RowsAffected() (int64, error) {
	return 1, nil
}
