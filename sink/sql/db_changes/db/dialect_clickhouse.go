package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math/big"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	clickhouse "github.com/AfterShip/clickhouse-sql-parser/parser"
	_ "github.com/ClickHouse/clickhouse-go/v2"
	"github.com/streamingfast/cli"
	sink "github.com/streamingfast/substreams/sink"
	"go.uber.org/zap"
	"golang.org/x/exp/maps"
)

type ClickhouseDialect struct {
	cursorTableName string
	cluster         string
	schemaName      string
}

func NewClickhouseDialect(schemaName string, cursorTableName string, cluster string) *ClickhouseDialect {
	return &ClickhouseDialect{
		cursorTableName: cursorTableName,
		cluster:         cluster,
		schemaName:      schemaName,
	}
}

// Clickhouse should be used to insert a lot of data in batches. The current official clickhouse
// driver doesn't support Transactions for multiple tables. The only way to add in batches is
// creating a transaction for a table, adding all rows and commiting it.
func (d ClickhouseDialect) Flush(tx Tx, ctx context.Context, l *Loader, outputModuleHash string, lastFinalBlock uint64) (int, error) {
	var entryCount int
	for entriesPair := l.entries.Oldest(); entriesPair != nil; entriesPair = entriesPair.Next() {
		tableName := entriesPair.Key
		entries := entriesPair.Value
		tx, err := l.DB.BeginTx(ctx, nil)
		if err != nil {
			return entryCount, fmt.Errorf("failed to begin db transaction")
		}

		if l.tracer.Enabled() {
			l.logger.Debug("flushing table entries", zap.String("table_name", tableName), zap.Int("entry_count", entries.Len()))
		}
		info := l.tables[tableName]
		columns := make([]string, 0, len(info.columnsByName))
		for column := range info.columnsByName {
			columns = append(columns, column)
		}
		sort.Strings(columns)
		query := fmt.Sprintf(
			"INSERT INTO %s.%s (%s)",
			EscapeIdentifier(d.schemaName),
			EscapeIdentifier(tableName),
			strings.Join(columns, ","))
		batch, err := tx.Prepare(query)
		if err != nil {
			return entryCount, fmt.Errorf("failed to prepare insert into %q: %w", tableName, err)
		}
		for entryPair := entries.Oldest(); entryPair != nil; entryPair = entryPair.Next() {
			entry := entryPair.Value

			if l.tracer.Enabled() {
				l.logger.Debug("adding query from operation to transaction", zap.Stringer("op", entry), zap.String("query", query))
			}

			values, err := convertOpToClickhouseValues(entry)
			if err != nil {
				return entryCount, fmt.Errorf("failed to get values: %w", err)
			}

			if _, err := batch.ExecContext(ctx, values...); err != nil {
				return entryCount, fmt.Errorf("executing for entry %q: %w", values, err)
			}
		}

		if err := tx.Commit(); err != nil {
			return entryCount, fmt.Errorf("failed to commit db transaction: %w", err)
		}
		entryCount += entries.Len()
	}

	return entryCount, nil
}

func (d ClickhouseDialect) Revert(tx Tx, ctx context.Context, l *Loader, lastValidFinalBlock uint64) error {
	return fmt.Errorf("clickhouse driver does not support reorg management.")
}

func (d ClickhouseDialect) GetCreateCursorQuery(schema string, withPostgraphile bool) string {
	_ = withPostgraphile // TODO: see if this can work

	clusterClause := ""
	engine := "ReplacingMergeTree()"
	if d.cluster != "" {
		clusterClause = fmt.Sprintf("ON CLUSTER %s", EscapeIdentifier(d.cluster))
		engine = "ReplicatedReplacingMergeTree()"
	}

	return fmt.Sprintf(cli.Dedent(`
	CREATE TABLE IF NOT EXISTS %s.%s %s
	(
    id         String,
		cursor     String,
		block_num  Int64,
		block_id   String
	) Engine = %s ORDER BY id;
	`), EscapeIdentifier(schema), EscapeIdentifier(d.cursorTableName), clusterClause, engine)
}

func (d ClickhouseDialect) GetCreateHistoryQuery(schema string, withPostgraphile bool) string {
	panic("clickhouse does not support reorg management")
}

func (d ClickhouseDialect) ExecuteSetupScript(ctx context.Context, l *Loader, schemaSql string) error {
	if d.schemaName != "default" {
		useDbQuery := fmt.Sprintf("USE %s", EscapeIdentifier(d.schemaName))
		if _, err := l.ExecContext(ctx, useDbQuery); err != nil {
			l.logger.Error("failed to switch to database", zap.String("database", d.schemaName), zap.Error(err))
			return fmt.Errorf("use database %s: %w", d.schemaName, err)
		}
	}

	if d.cluster != "" {
		stmts, err := clickhouse.NewParser(schemaSql).ParseStmts()
		if err != nil {
			return fmt.Errorf("parsing schemaName: %w", err)
		}

		for _, stmt := range stmts {
			if createDatabase, ok := stmt.(*clickhouse.CreateDatabase); ok {
				l.logger.Debug("appending 'ON CLUSTER' clause to 'CREATE DATABASE'", zap.String("cluster", d.cluster), zap.Stringer("database", createDatabase.Name))
				createDatabase.OnCluster = &clickhouse.ClusterClause{Expr: &clickhouse.StringLiteral{Literal: d.cluster}}
			}
			if createTable, ok := stmt.(*clickhouse.CreateTable); ok {
				l.logger.Debug("appending 'ON CLUSTER' clause to 'CREATE TABLE'", zap.String("cluster", d.cluster), zap.String("table", createTable.Name.String()))
				createTable.OnCluster = &clickhouse.ClusterClause{Expr: &clickhouse.StringLiteral{Literal: d.cluster}}

				if !strings.HasPrefix(createTable.Engine.Name, "Replicated") &&
					strings.HasSuffix(createTable.Engine.Name, "MergeTree") {
					newEngine := "Replicated" + createTable.Engine.Name
					l.logger.Debug("replacing table engine with replicated one", zap.String("table", createTable.Name.String()), zap.String("engine", createTable.Engine.Name), zap.String("new_engine", newEngine))
					createTable.Engine.Name = newEngine
				}
			}
			if createMaterializedView, ok := stmt.(*clickhouse.CreateMaterializedView); ok {
				l.logger.Debug("appending 'ON CLUSTER' clause to 'CREATE MATERIALIZED VIEW'", zap.String("cluster", d.cluster), zap.Stringer("materialized_view", createMaterializedView.Name))
				createMaterializedView.OnCluster = &clickhouse.ClusterClause{Expr: &clickhouse.StringLiteral{Literal: d.cluster}}

				if createMaterializedView.Engine != nil && !strings.HasPrefix(createMaterializedView.Engine.Name, "Replicated") &&
					strings.HasSuffix(createMaterializedView.Engine.Name, "MergeTree") {
					newEngine := "Replicated" + createMaterializedView.Engine.Name
					l.logger.Debug("replacing table engine with replicated one", zap.Stringer("materialized_view", createMaterializedView.Name), zap.String("engine", createMaterializedView.Engine.Name), zap.String("new_engine", newEngine))
					createMaterializedView.Engine.Name = newEngine
				}
			}
			if createView, ok := stmt.(*clickhouse.CreateView); ok {
				l.logger.Debug("appending 'ON CLUSTER' clause to 'CREATE VIEW'", zap.String("cluster", d.cluster), zap.Stringer("view", createView.Name))
				createView.OnCluster = &clickhouse.ClusterClause{Expr: &clickhouse.StringLiteral{Literal: d.cluster}}
			}
			if createFunction, ok := stmt.(*clickhouse.CreateFunction); ok {
				l.logger.Debug("appending 'ON CLUSTER' clause to 'CREATE FUNCTION'", zap.String("cluster", d.cluster), zap.Stringer("function", createFunction.FunctionName))
				createFunction.OnCluster = &clickhouse.ClusterClause{Expr: &clickhouse.StringLiteral{Literal: d.cluster}}
			}

			if _, err := l.ExecContext(ctx, stmt.String()); err != nil {
				l.logger.Error("failed to execute schema statement", zap.String("statement", stmt.String()), zap.Error(err))
				return fmt.Errorf("exec clickhouse cluster statements: %w", err)
			}
		}
	} else {
		// Splitting statements by ';' is not perfect but should be enough for now,
		// it will fail for example if user enter a string that contains a ;!
		for query := range strings.SplitSeq(schemaSql, ";") {
			if len(strings.TrimSpace(query)) == 0 {
				continue
			}
			if _, err := l.ExecContext(ctx, query); err != nil {
				return fmt.Errorf("exec clickhouse statements: %w", err)
			}
		}
	}

	return nil
}

func (d ClickhouseDialect) GetUpdateCursorQuery(table, moduleHash string, cursor *sink.Cursor, block_num uint64, block_id string) string {
	return query(`
			INSERT INTO %s (id, cursor, block_num, block_id) values ('%s', '%s', %d, '%s')
	`, table, moduleHash, cursor, block_num, block_id)
}

func (d ClickhouseDialect) GetAllCursorsQuery(table string) string {
	return fmt.Sprintf("SELECT id, cursor, block_num, block_id FROM %s FINAL", table)
}

func (d ClickhouseDialect) ParseDatetimeNormalization(value string) string {
	return fmt.Sprintf("parseDateTimeBestEffort(%s)", escapeStringValue(value))
}

func (d ClickhouseDialect) DriverSupportRowsAffected() bool {
	return false
}

func (d ClickhouseDialect) OnlyInserts() bool {
	return true
}

func (d ClickhouseDialect) AllowPkDuplicates() bool {
	return true
}

func (d ClickhouseDialect) CreateUser(tx Tx, ctx context.Context, l *Loader, username string, password string, _database string, readOnly bool) error {
	user, pass := EscapeIdentifier(username), escapeStringValue(password)

	onClusterClause := ""
	if d.cluster != "" {
		onClusterClause = fmt.Sprintf("ON CLUSTER %s", EscapeIdentifier(d.cluster))
	}

	createUserQ := fmt.Sprintf("CREATE USER IF NOT EXISTS %s %s IDENTIFIED WITH plaintext_password BY %s;", user, onClusterClause, pass)
	_, err := tx.ExecContext(ctx, createUserQ)
	if err != nil {
		return fmt.Errorf("executing create user query %q: %w", createUserQ, err)
	}

	var grantQ string
	if readOnly {
		grantQ = fmt.Sprintf(`
            GRANT %s SELECT ON *.* TO %s;
        `, onClusterClause, user)
	} else {
		grantQ = fmt.Sprintf(`
            GRANT %s ALL ON *.* TO %s;
        `, onClusterClause, user)
	}

	_, err = tx.ExecContext(ctx, grantQ)
	if err != nil {
		return fmt.Errorf("executing grant query %q: %w", grantQ, err)
	}

	return nil
}

func convertOpToClickhouseValues(o *Operation) ([]any, error) {
	columns := make([]string, len(o.data))
	i := 0
	for column := range o.data {
		columns[i] = column
		i++
	}
	sort.Strings(columns)
	values := make([]any, len(o.data))
	for i, v := range columns {
		if col, exists := o.table.columnsByName[v]; exists {
			fieldData := o.data[v]
			convertedType, err := convertToType(fieldData.Value, col.scanType)
			if err != nil {
				return nil, fmt.Errorf("converting value %q to type %q in column %q: %w", fieldData.Value, col.scanType, v, err)
			}
			values[i] = convertedType
		} else {
			return nil, fmt.Errorf("cannot find column %q for table %q (valid columns are %q)", v, o.table.identifier, strings.Join(maps.Keys(o.table.columnsByName), ", "))
		}
	}
	return values, nil
}

func convertToType(value string, valueType reflect.Type) (any, error) {
	switch valueType.Kind() {
	case reflect.String:
		return value, nil
	case reflect.Slice:
		if valueType.Elem().Kind() == reflect.Struct || valueType.Elem().Kind() == reflect.Ptr {
			return nil, fmt.Errorf("%q is not supported as Clickhouse Array type", valueType.Elem().Name())
		}

		res := reflect.New(reflect.SliceOf(valueType.Elem()))
		if err := json.Unmarshal([]byte(value), res.Interface()); err != nil {
			return "", fmt.Errorf("could not JSON unmarshal slice value %q: %w", value, err)
		}

		return res.Elem().Interface(), nil
	case reflect.Bool:
		return strconv.ParseBool(value)
	case reflect.Int:
		v, err := strconv.ParseInt(value, 10, 0)
		return int(v), err
	case reflect.Int8:
		v, err := strconv.ParseInt(value, 10, 8)
		return int8(v), err
	case reflect.Int16:
		v, err := strconv.ParseInt(value, 10, 16)
		return int16(v), err
	case reflect.Int32:
		v, err := strconv.ParseInt(value, 10, 32)
		return int32(v), err
	case reflect.Int64:
		return strconv.ParseInt(value, 10, 64)
	case reflect.Uint:
		v, err := strconv.ParseUint(value, 10, 0)
		return uint(v), err
	case reflect.Uint8:
		v, err := strconv.ParseUint(value, 10, 8)
		return uint8(v), err
	case reflect.Uint16:
		v, err := strconv.ParseUint(value, 10, 16)
		return uint16(v), err
	case reflect.Uint32:
		v, err := strconv.ParseUint(value, 10, 32)
		return uint32(v), err
	case reflect.Uint64:
		return strconv.ParseUint(value, 10, 0)
	case reflect.Float32, reflect.Float64:
		return strconv.ParseFloat(value, 10)
	case reflect.Struct:
		if valueType == reflectTypeTime {
			if integerRegex.MatchString(value) {
				i, err := strconv.Atoi(value)
				if err != nil {
					return "", fmt.Errorf("could not convert %s to int: %w", value, err)
				}

				return int64(i), nil
			}

			var v time.Time
			var err error
			if strings.Contains(value, "T") && strings.HasSuffix(value, "Z") {
				v, err = time.Parse("2006-01-02T15:04:05Z", value)
			} else if dateRegex.MatchString(value) {
				// This is a Clickhouse Date field. The Clickhouse Go client doesn't convert unix timestamp into Date,
				// so we just validate the format here and return a string.
				_, err = time.Parse("2006-01-02", value)
				if err != nil {
					return "", fmt.Errorf("could not convert %s to date: %w", value, err)
				}
				return value, nil
			} else {
				v, err = time.Parse("2006-01-02 15:04:05", value)
			}
			if err != nil {
				return "", fmt.Errorf("could not convert %s to time: %w", value, err)
			}
			return v.Unix(), nil
		}
		return "", fmt.Errorf("unsupported struct type %s", valueType)

	case reflect.Ptr:
		if valueType.String() == "*big.Int" {
			newInt := new(big.Int)
			newInt.SetString(value, 10)
			return newInt, nil
		}

		elemType := valueType.Elem()
		val, err := convertToType(value, elemType)
		if err != nil {
			return nil, fmt.Errorf("invalid pointer type: %w", err)
		}

		// We cannot just return &val here as this will return an *interface{} that the Clickhouse Go client won't be
		// able to convert on inserting. Instead, we create a new variable using the type that valueType has been
		// pointing to, assign the converted value from convertToType to that and then return a pointer to the new variable.
		result := reflect.New(elemType).Elem()
		result.Set(reflect.ValueOf(val))
		return result.Addr().Interface(), nil

	default:
		return value, nil
	}
}

func (d ClickhouseDialect) GetTableColumns(db *sql.DB, schemaName, tableName string) ([]*sql.ColumnType, error) {
	// For TCP, use DESCRIBE TABLE to filter out AggregateFunction columns
	describeQuery := fmt.Sprintf("DESCRIBE TABLE %s.%s",
		EscapeIdentifier(schemaName),
		EscapeIdentifier(tableName))

	describeRows, err := db.Query(describeQuery)
	if err != nil {
		return nil, fmt.Errorf("describing table structure: %w", err)
	}
	defer describeRows.Close()

	var nonAggregateColumns []string

	// Get the column types to know how many columns DESCRIBE returns
	describeColumnTypes, err := describeRows.ColumnTypes()
	if err != nil {
		return nil, fmt.Errorf("getting describe column types: %w", err)
	}

	// Parse DESCRIBE results to filter out AggregateFunction columns
	for describeRows.Next() {
		// Create slice to hold all column values dynamically
		values := make([]interface{}, len(describeColumnTypes))
		valuePtrs := make([]interface{}, len(describeColumnTypes))
		for i := range values {
			valuePtrs[i] = &values[i]
		}

		err := describeRows.Scan(valuePtrs...)
		if err != nil {
			return nil, fmt.Errorf("scanning describe results: %w", err)
		}

		// First column is always the column name, second is the data type,
		// third is the default_type (MATERIALIZED, ALIAS, DEFAULT, or empty)
		name := fmt.Sprintf("%v", values[0])
		dataType := fmt.Sprintf("%v", values[1])
		defaultType := ""
		if len(values) > 2 && values[2] != nil {
			defaultType = fmt.Sprintf("%v", values[2])
		}

		// Skip AggregateFunction columns and MATERIALIZED columns
		// MATERIALIZED columns are auto-computed and cannot be inserted into
		if !strings.Contains(dataType, "AggregateFunction") && defaultType != "MATERIALIZED" {
			nonAggregateColumns = append(nonAggregateColumns, EscapeIdentifier(name))
		}
	}

	if err := describeRows.Err(); err != nil {
		return nil, fmt.Errorf("iterating describe results: %w", err)
	}

	if len(nonAggregateColumns) == 0 {
		return nil, fmt.Errorf("no non-aggregate columns found in table %s.%s", schemaName, tableName)
	}

	// TCP protocol works well with WHERE 1=0
	columnList := strings.Join(nonAggregateColumns, ", ")
	selectQuery := fmt.Sprintf("SELECT %s FROM %s.%s WHERE 1=0",
		columnList,
		EscapeIdentifier(schemaName),
		EscapeIdentifier(tableName))

	rows, err := db.Query(selectQuery)
	if err != nil {
		return nil, fmt.Errorf("querying filtered table structure: %w", err)
	}
	defer rows.Close()

	return rows.ColumnTypes()
}

func (d ClickhouseDialect) GetTablesInSchema(db *sql.DB, schemaName string) ([][2]string, error) {
	// Use system.tables to query for tables in the schema
	// Filter out MaterializedView as they are not regular tables and should not receive direct inserts
	query := fmt.Sprintf(`
		SELECT database AS table_schema, name AS table_name
		FROM system.tables
		WHERE database = '%s'
		  AND NOT is_temporary
		  AND engine NOT LIKE '%%View'
		  AND engine NOT LIKE 'System%%'
		  AND has_own_data != 0
		ORDER BY database, name
	`, schemaName)

	rows, err := db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("querying tables from system.tables: %w", err)
	}
	defer rows.Close()

	var result [][2]string
	for rows.Next() {
		var schema, table string
		if err := rows.Scan(&schema, &table); err != nil {
			return nil, fmt.Errorf("scanning table row: %w", err)
		}
		result = append(result, [2]string{schema, table})
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating table rows: %w", err)
	}

	return result, nil
}

const clickhousePrimaryKeyQuery = `
	SELECT name
	FROM system.columns
	WHERE database = %s
		AND table = %s
		AND is_in_primary_key
	ORDER BY position DESC`

func (d ClickhouseDialect) GetPrimaryKey(db *sql.DB, schemaName, tableName string) ([]string, error) {
	var query string
	var args []interface{}

	if schemaName == "" {
		query = fmt.Sprintf(clickhousePrimaryKeyQuery, "currentDatabase()", "?")
		args = []interface{}{tableName}
	} else {
		query = fmt.Sprintf(clickhousePrimaryKeyQuery, "?", "?")
		args = []interface{}{schemaName, tableName}
	}

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("querying primary key: %w", err)
	}
	defer rows.Close()

	var columns []string
	for rows.Next() {
		var column string
		if err := rows.Scan(&column); err != nil {
			return nil, fmt.Errorf("scanning primary key column: %w", err)
		}
		columns = append(columns, column)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating primary key rows: %w", err)
	}

	return columns, nil
}
