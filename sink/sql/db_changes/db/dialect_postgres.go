package db

import (
	"cmp"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/streamingfast/cli"
	sink "github.com/streamingfast/substreams/sink"
	"go.uber.org/zap"
	"golang.org/x/exp/maps"
)

type PostgresDialect struct {
	cursorTableName  string
	historyTableName string
	schemaName       string
}

func NewPostgresDialect(schemaName string, cursorTableName string, historyTableName string) *PostgresDialect {
	return &PostgresDialect{
		cursorTableName:  cursorTableName,
		historyTableName: historyTableName,
		schemaName:       schemaName,
	}
}

func (d PostgresDialect) Revert(tx Tx, ctx context.Context, l *Loader, lastValidFinalBlock uint64) error {
	query := fmt.Sprintf(`SELECT op,table_name,pk,prev_value,block_num FROM %s WHERE "block_num" > %d ORDER BY "block_num" DESC`,
		d.historyTable(d.schemaName),
		lastValidFinalBlock,
	)

	rows, err := tx.QueryContext(ctx, query)
	if err != nil {
		return err
	}

	var reversions []func() error
	l.logger.Info("reverting forked block block(s)", zap.Uint64("last_valid_final_block", lastValidFinalBlock))
	if rows != nil { // rows will be nil with no error only in testing scenarios
		defer rows.Close()
		for rows.Next() {
			var op string
			var table_name string
			var pk string
			var prev_value_nullable sql.NullString
			var block_num uint64
			if err := rows.Scan(&op, &table_name, &pk, &prev_value_nullable, &block_num); err != nil {
				return fmt.Errorf("scanning row: %w", err)
			}
			l.logger.Debug("reverting", zap.String("operation", op), zap.String("table_name", table_name), zap.String("pk", pk), zap.Uint64("block_num", block_num))
			prev_value := prev_value_nullable.String

			// we can't call revertOp inside this loop, because it calls tx.ExecContext,
			// which can't run while this query is "active" or it will silently discard the remaining rows!
			reversions = append(reversions, func() error {
				if err := d.revertOp(tx, ctx, op, table_name, pk, prev_value, block_num); err != nil {
					return fmt.Errorf("revertOp: %w", err)
				}
				return nil
			})
		}
		if err := rows.Err(); err != nil {
			return fmt.Errorf("iterating on rows from query %q: %w", query, err)
		}
		for _, reversion := range reversions {
			if err := reversion(); err != nil {
				return fmt.Errorf("execution revert operation: %w", err)
			}
		}
	}
	pruneHistory := fmt.Sprintf(`DELETE FROM %s WHERE "block_num" > %d;`,
		d.historyTable(d.schemaName),
		lastValidFinalBlock,
	)

	_, err = tx.ExecContext(ctx, pruneHistory)
	if err != nil {
		return fmt.Errorf("executing pruneHistory: %w", err)
	}
	return nil
}

func (d PostgresDialect) Flush(tx Tx, ctx context.Context, l *Loader, outputModuleHash string, lastFinalBlock uint64) (int, error) {
	var totalRows int
	for entriesPair := l.entries.Oldest(); entriesPair != nil; entriesPair = entriesPair.Next() {
		entries := entriesPair.Value
		totalRows += entries.Len()

		if l.tracer.Enabled() {
			l.logger.Debug("flushing table rows", zap.String("table_name", entriesPair.Key), zap.Int("row_count", entries.Len()))
		}
	}

	allOperations := make([]*Operation, 0, totalRows)
	for entriesPair := l.entries.Oldest(); entriesPair != nil; entriesPair = entriesPair.Next() {
		entries := entriesPair.Value
		for entryPair := entries.Oldest(); entryPair != nil; entryPair = entryPair.Next() {
			allOperations = append(allOperations, entryPair.Value)
		}
	}

	slices.SortFunc(allOperations, func(a, b *Operation) int {
		return cmp.Compare(a.ordinal, b.ordinal)
	})

	var rowCount int
	for _, entry := range allOperations {
		normalQuery, undoQuery, err := d.prepareStatement(d.schemaName, entry)
		if err != nil {
			return 0, fmt.Errorf("failed to prepare statement: %w", err)
		}

		// Execute undo query first (if present) to save state before modifying
		if undoQuery != "" {
			if l.tracer.Enabled() {
				l.logger.Debug("adding undo query from operation to transaction", zap.Stringer("op", entry), zap.String("query", undoQuery), zap.Uint64("ordinal", entry.ordinal))
			}

			undoStart := time.Now()
			if _, err := tx.ExecContext(ctx, undoQuery); err != nil {
				return 0, fmt.Errorf("executing undo query %q: %w", undoQuery, err)
			}
			undoDuration := time.Since(undoStart)
			QueryExecutionDuration.AddInt64(undoDuration.Nanoseconds(), "undo")
		}

		// Execute normal query
		if l.tracer.Enabled() {
			l.logger.Debug("adding normal query from operation to transaction", zap.Stringer("op", entry), zap.String("query", normalQuery), zap.Uint64("ordinal", entry.ordinal))
		}

		normalStart := time.Now()
		if _, err := tx.ExecContext(ctx, normalQuery); err != nil {
			return 0, fmt.Errorf("executing normal query %q: %w", normalQuery, err)
		}
		normalDuration := time.Since(normalStart)
		QueryExecutionDuration.AddInt64(normalDuration.Nanoseconds(), "normal")

		rowCount++
	}

	pruneStart := time.Now()
	if err := d.pruneReversibleSegment(tx, ctx, d.schemaName, lastFinalBlock); err != nil {
		return 0, err
	}
	pruneDuration := time.Since(pruneStart)
	PruneReversibleSegmentDuration.AddInt64(pruneDuration.Nanoseconds())

	return rowCount, nil
}

func (d PostgresDialect) revertOp(tx Tx, ctx context.Context, op, escaped_table_name, pk, prev_value string, block_num uint64) error {

	pkmap := make(map[string]string)
	if err := json.Unmarshal([]byte(pk), &pkmap); err != nil {
		return fmt.Errorf("revertOp: unmarshalling %q: %w", pk, err)
	}
	switch op {
	case "I":
		query := fmt.Sprintf(`DELETE FROM %s WHERE %s;`,
			escaped_table_name,
			getPrimaryKeyWhereClause(pkmap, ""),
		)
		if _, err := tx.ExecContext(ctx, query); err != nil {
			return fmt.Errorf("executing revert query %q: %w", query, err)
		}
	case "D":
		query := fmt.Sprintf(`INSERT INTO %s SELECT * FROM json_populate_record(null::%s,%s);`,
			escaped_table_name,
			escaped_table_name,
			escapeStringValue(prev_value),
		)
		if _, err := tx.ExecContext(ctx, query); err != nil {
			return fmt.Errorf("executing revert query %q: %w", query, err)
		}

	case "U":
		columns, err := sqlColumnNamesFromJSON(prev_value)
		if err != nil {
			return err
		}

		query := fmt.Sprintf(`UPDATE %s SET(%s)=((SELECT %s FROM json_populate_record(null::%s,%s))) WHERE %s;`,
			escaped_table_name,
			columns,
			columns,
			escaped_table_name,
			escapeStringValue(prev_value),
			getPrimaryKeyWhereClause(pkmap, ""),
		)
		if _, err := tx.ExecContext(ctx, query); err != nil {
			return fmt.Errorf("executing revert query %q: %w", query, err)
		}
	default:
		panic("invalid op in revert command")
	}
	return nil
}

func sqlColumnNamesFromJSON(in string) (string, error) {
	valueMap := make(map[string]interface{})
	if err := json.Unmarshal([]byte(in), &valueMap); err != nil {
		return "", fmt.Errorf("unmarshalling %q into valueMap: %w", in, err)
	}
	escapedNames := make([]string, len(valueMap))
	i := 0
	for k := range valueMap {
		escapedNames[i] = EscapeIdentifier(k)
		i++
	}
	sort.Strings(escapedNames)

	return strings.Join(escapedNames, ","), nil
}

func (d PostgresDialect) pruneReversibleSegment(tx Tx, ctx context.Context, schema string, highestFinalBlock uint64) error {
	query := fmt.Sprintf(`DELETE FROM %s WHERE block_num <= %d;`, d.historyTable(schema), highestFinalBlock)
	if _, err := tx.ExecContext(ctx, query); err != nil {
		return fmt.Errorf("executing prune query %q: %w", query, err)
	}
	return nil
}

func (d PostgresDialect) GetCreateCursorQuery(schema string, withPostgraphile bool) string {
	out := fmt.Sprintf(cli.Dedent(`
		create table if not exists %s.%s
		(
			id         text not null constraint %s primary key,
			cursor     text,
			block_num  bigint,
			block_id   text
		);
		`), EscapeIdentifier(schema), EscapeIdentifier(d.cursorTableName), EscapeIdentifier(d.cursorTableName+"_pk"))
	if withPostgraphile {
		out += fmt.Sprintf("COMMENT ON TABLE %s.%s IS E'@omit';",
			EscapeIdentifier(schema), EscapeIdentifier(d.cursorTableName))
	}
	return out
}

func (d PostgresDialect) GetCreateHistoryQuery(schema string, withPostgraphile bool) string {
	out := fmt.Sprintf(cli.Dedent(`
		create table if not exists %s
		(
            id           SERIAL PRIMARY KEY,
            op           char,
            table_name   text,
			pk           text,
            prev_value   text,
			block_num    bigint
		);
		`),
		d.historyTable(schema),
	)
	if withPostgraphile {
		out += fmt.Sprintf("COMMENT ON TABLE %s.%s IS E'@omit';",
			EscapeIdentifier(schema), EscapeIdentifier(d.historyTableName))
	}
	return out
}

func (d PostgresDialect) ExecuteSetupScript(ctx context.Context, l *Loader, schemaSql string) error {
	// Prepend search_path directive to ensure user SQL runs in the correct schema context
	fullSql := fmt.Sprintf(`SET search_path TO %s;`+"\n\n%s", EscapeIdentifier(d.schemaName), schemaSql)

	if _, err := l.ExecContext(ctx, fullSql); err != nil {
		return fmt.Errorf("exec postgres statements: %w", err)
	}
	return nil
}

func (d PostgresDialect) GetUpdateCursorQuery(table, moduleHash string, cursor *sink.Cursor, block_num uint64, block_id string) string {
	return query(`
		UPDATE %s set cursor = '%s', block_num = %d, block_id = '%s' WHERE id = '%s';
	`, table, cursor, block_num, block_id, moduleHash)
}

func (d PostgresDialect) GetAllCursorsQuery(table string) string {
	return fmt.Sprintf("SELECT id, cursor, block_num, block_id FROM %s", table)
}

func (d PostgresDialect) ParseDatetimeNormalization(value string) string {
	return escapeStringValue(value)
}

func (d PostgresDialect) DriverSupportRowsAffected() bool {
	return true
}

func (d PostgresDialect) OnlyInserts() bool {
	return false
}

func (d PostgresDialect) AllowPkDuplicates() bool {
	return false
}

func (d PostgresDialect) CreateUser(tx Tx, ctx context.Context, l *Loader, username string, password string, database string, readOnly bool) error {
	user, pass, db := EscapeIdentifier(username), password, EscapeIdentifier(database)
	var q string
	if readOnly {
		q = fmt.Sprintf(`
            CREATE ROLE %s LOGIN PASSWORD '%s';
            GRANT CONNECT ON DATABASE %s TO %s;
            GRANT USAGE ON SCHEMA public TO %s;
            ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT SELECT ON TABLES TO %s;
            GRANT SELECT ON ALL TABLES IN SCHEMA public TO %s;
        `, user, pass, db, user, user, user, user)
	} else {
		q = fmt.Sprintf("CREATE USER %s WITH PASSWORD '%s'; GRANT ALL PRIVILEGES ON DATABASE %s TO %s;", user, pass, db, user)
	}

	_, err := tx.ExecContext(ctx, q)
	if err != nil {
		return fmt.Errorf("executing create user query %q: %w", q, err)
	}

	return nil
}

func (d PostgresDialect) historyTable(schema string) string {
	return fmt.Sprintf("%s.%s", EscapeIdentifier(schema), EscapeIdentifier(d.historyTableName))
}

func (d PostgresDialect) saveInsert(schema string, table string, primaryKey map[string]string, blockNum uint64) string {
	return fmt.Sprintf(`INSERT INTO %s (op,table_name,pk,block_num) values (%s,%s,%s,%d);`,
		d.historyTable(schema),
		escapeStringValue("I"),
		escapeStringValue(table),
		escapeStringValue(primaryKeyToJSON(primaryKey)),
		blockNum,
	)
}

/*
with t as (select 'default' id)
select CASE WHEN block_meta.id is null THEN 'I' ELSE 'U' END AS op, '"public"."block_meta"', 'allo', row_to_json(block_meta),10  from t left join block_meta on block_meta.id='default';
*/
func (d PostgresDialect) saveUpsert(schema string, escapedTableName string, primaryKey map[string]string, blockNum uint64) string {
	schemaAndTable := fmt.Sprintf("%s.%s", EscapeIdentifier(schema), escapedTableName)

	return fmt.Sprintf(`
		WITH t as (select %s)
		INSERT INTO %s (op,table_name,pk,prev_value,block_num)
		SELECT CASE WHEN %s THEN 'I' ELSE 'U' END AS op, %s, %s, row_to_json(%s),%d from t left join %s.%s on %s;`,

		getPrimaryKeyFakeEmptyValues(primaryKey),
		d.historyTable(schema),

		getPrimaryKeyFakeEmptyValuesAssertion(primaryKey, escapedTableName),

		escapeStringValue(schemaAndTable), escapeStringValue(primaryKeyToJSON(primaryKey)), escapedTableName, blockNum,
		EscapeIdentifier(schema), escapedTableName,
		getPrimaryKeyWhereClause(primaryKey, escapedTableName),
	)

}

func (d PostgresDialect) saveUpdate(schema string, escapedTableName string, primaryKey map[string]string, blockNum uint64) string {
	return d.saveRow("U", schema, escapedTableName, primaryKey, blockNum)
}

func (d PostgresDialect) saveDelete(schema string, escapedTableName string, primaryKey map[string]string, blockNum uint64) string {
	return d.saveRow("D", schema, escapedTableName, primaryKey, blockNum)
}

func (d PostgresDialect) saveRow(op, schema, escapedTableName string, primaryKey map[string]string, blockNum uint64) string {
	schemaAndTable := fmt.Sprintf("%s.%s", EscapeIdentifier(schema), escapedTableName)
	return fmt.Sprintf(`INSERT INTO %s (op,table_name,pk,prev_value,block_num) SELECT %s,%s,%s,row_to_json(%s),%d FROM %s.%s WHERE %s;`,
		d.historyTable(schema),
		escapeStringValue(op), escapeStringValue(schemaAndTable), escapeStringValue(primaryKeyToJSON(primaryKey)), escapedTableName, blockNum,
		EscapeIdentifier(schema), escapedTableName,
		getPrimaryKeyWhereClause(primaryKey, ""),
	)

}

// getResultCast returns the appropriate cast suffix for the result of arithmetic operations
// based on the column's scan type. TEXT columns need ::text cast, numeric types don't need cast.
func getResultCast(scanType reflect.Type) string {
	if scanType == nil {
		return "" // unknown type, let PostgreSQL handle it
	}
	switch scanType.Kind() {
	case reflect.String:
		return "::text" // TEXT columns need explicit cast from numeric
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return "" // numeric types don't need cast, PostgreSQL will handle it
	default:
		return "" // unknown type, let PostgreSQL handle it
	}
}

func (d *PostgresDialect) prepareStatement(schema string, o *Operation) (normalQuery string, undoQuery string, err error) {
	var columns, values []string
	var updateOps []UpdateOp
	var scanTypes []reflect.Type
	if o.opType == OperationTypeInsert || o.opType == OperationTypeUpsert || o.opType == OperationTypeUpdate {
		columns, values, updateOps, scanTypes, err = d.prepareColValues(o.table, o.data)
		if err != nil {
			return "", "", fmt.Errorf("preparing column & values: %w", err)
		}
	}

	if o.opType == OperationTypeUpsert || o.opType == OperationTypeUpdate || o.opType == OperationTypeDelete {
		// A table without a primary key set yield a `primaryKey` map with a single entry where the key is an empty string
		if _, found := o.primaryKey[""]; found {
			return "", "", fmt.Errorf("trying to perform %s operation but table %q don't have a primary key set, this is not accepted", o.opType, o.table.name)
		}
	}

	switch o.opType {
	case OperationTypeInsert:
		insertQuery := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s);",
			o.table.identifier,
			strings.Join(columns, ","),
			strings.Join(values, ","),
		)

		if o.reversibleBlockNum != nil {
			return insertQuery, d.saveInsert(schema, o.table.identifier, o.primaryKey, *o.reversibleBlockNum), nil
		}
		return insertQuery, "", nil

	case OperationTypeUpsert:
		// Build per-field update expressions based on UpdateOp
		updates := make([]string, len(columns))
		for i := range columns {
			col := columns[i]
			resultCast := getResultCast(scanTypes[i])
			switch updateOps[i] {
			case UpdateOpSet:
				// Direct assignment: col = EXCLUDED.col
				updates[i] = fmt.Sprintf("%s=EXCLUDED.%s", col, col)
			case UpdateOpAdd:
				// Accumulate: col = COALESCE(col, 0) + EXCLUDED.col
				updates[i] = fmt.Sprintf("%s=(COALESCE(%s.%s::numeric, 0) + EXCLUDED.%s::numeric)%s", col, o.table.nameEscaped, col, col, resultCast)
			case UpdateOpMax:
				// Maximum: col = GREATEST(COALESCE(col, 0), EXCLUDED.col)
				updates[i] = fmt.Sprintf("%s=GREATEST(COALESCE(%s.%s::numeric, 0), EXCLUDED.%s::numeric)%s", col, o.table.nameEscaped, col, col, resultCast)
			case UpdateOpMin:
				// Minimum: col = LEAST(COALESCE(col, 0), EXCLUDED.col)
				updates[i] = fmt.Sprintf("%s=LEAST(COALESCE(%s.%s::numeric, 0), EXCLUDED.%s::numeric)%s", col, o.table.nameEscaped, col, col, resultCast)
			case UpdateOpSetIfNull:
				// Set only if NULL (first value wins): col = COALESCE(col, EXCLUDED.col)
				updates[i] = fmt.Sprintf("%s=COALESCE(%s.%s, EXCLUDED.%s)", col, o.table.nameEscaped, col, col)
			default:
				updates[i] = fmt.Sprintf("%s=EXCLUDED.%s", col, col)
			}
		}

		// Escape primary key column names to preserve case sensitivity (e.g., camelCase)
		escapedPKColumns := make([]string, 0, len(o.primaryKey))
		for pkColumn := range o.primaryKey {
			escapedPKColumns = append(escapedPKColumns, EscapeIdentifier(pkColumn))
		}
		sort.Strings(escapedPKColumns) // Sort for deterministic output

		insertQuery := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s) ON CONFLICT (%s) DO UPDATE SET %s;",
			o.table.identifier,
			strings.Join(columns, ","),
			strings.Join(values, ","),
			strings.Join(escapedPKColumns, ","),
			strings.Join(updates, ", "),
		)

		if o.reversibleBlockNum != nil {
			return insertQuery, d.saveUpsert(schema, o.table.nameEscaped, o.primaryKey, *o.reversibleBlockNum), nil
		}
		return insertQuery, "", nil

	case OperationTypeUpdate:
		// Build per-field update expressions based on UpdateOp
		updates := make([]string, len(columns))
		for i := range columns {
			col := columns[i]
			val := values[i]
			resultCast := getResultCast(scanTypes[i])
			switch updateOps[i] {
			case UpdateOpSet:
				// Direct assignment: col = value
				updates[i] = fmt.Sprintf("%s=%s", col, val)
			case UpdateOpAdd:
				// Accumulate: col = COALESCE(col, 0) + value
				updates[i] = fmt.Sprintf("%s=(COALESCE(%s::numeric, 0) + %s::numeric)%s", col, col, val, resultCast)
			case UpdateOpMax:
				// Maximum: col = GREATEST(COALESCE(col, 0), value)
				updates[i] = fmt.Sprintf("%s=GREATEST(COALESCE(%s::numeric, 0), %s::numeric)%s", col, col, val, resultCast)
			case UpdateOpMin:
				// Minimum: col = LEAST(COALESCE(col, 0), value)
				updates[i] = fmt.Sprintf("%s=LEAST(COALESCE(%s::numeric, 0), %s::numeric)%s", col, col, val, resultCast)
			case UpdateOpSetIfNull:
				// Set only if NULL (first value wins): col = COALESCE(col, value)
				updates[i] = fmt.Sprintf("%s=COALESCE(%s, %s)", col, col, val)
			default:
				updates[i] = fmt.Sprintf("%s=%s", col, val)
			}
		}

		primaryKeySelector := getPrimaryKeyWhereClause(o.primaryKey, "")

		updateQuery := fmt.Sprintf("UPDATE %s SET %s WHERE %s",
			o.table.identifier,
			strings.Join(updates, ", "),
			primaryKeySelector,
		)

		if o.reversibleBlockNum != nil {
			return updateQuery, d.saveUpdate(schema, o.table.nameEscaped, o.primaryKey, *o.reversibleBlockNum), nil
		}
		return updateQuery, "", nil

	case OperationTypeDelete:
		primaryKeyWhereClause := getPrimaryKeyWhereClause(o.primaryKey, "")
		deleteQuery := fmt.Sprintf("DELETE FROM %s WHERE %s",
			o.table.identifier,
			primaryKeyWhereClause,
		)
		if o.reversibleBlockNum != nil {
			return deleteQuery, d.saveDelete(schema, o.table.nameEscaped, o.primaryKey, *o.reversibleBlockNum), nil
		}
		return deleteQuery, "", nil

	default:
		panic(fmt.Errorf("unknown operation type %q", o.opType))
	}
}

func (d *PostgresDialect) prepareColValues(table *TableInfo, colValues map[string]FieldData) (columns []string, values []string, updateOps []UpdateOp, scanTypes []reflect.Type, err error) {
	if len(colValues) == 0 {
		return
	}

	columns = make([]string, len(colValues))
	values = make([]string, len(colValues))
	updateOps = make([]UpdateOp, len(colValues))
	scanTypes = make([]reflect.Type, len(colValues))

	i := 0
	for colName := range colValues {
		columns[i] = colName
		i++
	}
	sort.Strings(columns) // sorted for determinism in tests

	for i, columnName := range columns {
		fieldData := colValues[columnName]
		columnInfo, found := table.columnsByName[columnName]
		if !found {
			return nil, nil, nil, nil, fmt.Errorf("cannot find column %q for table %q (valid columns are %q)", columnName, table.identifier, strings.Join(maps.Keys(table.columnsByName), ", "))
		}

		normalizedValue, err := d.normalizeValueType(fieldData.Value, columnInfo.scanType)
		if err != nil {
			return nil, nil, nil, nil, fmt.Errorf("getting sql value from table %s for column %q raw value %q: %w", table.identifier, columnName, fieldData.Value, err)
		}

		values[i] = normalizedValue
		columns[i] = columnInfo.escapedName // escape the column name
		updateOps[i] = fieldData.UpdateOp
		scanTypes[i] = columnInfo.scanType
	}
	return
}

func getPrimaryKeyFakeEmptyValues(primaryKey map[string]string) string {
	if len(primaryKey) == 1 {
		for key := range primaryKey {
			return "'' " + EscapeIdentifier(key)
		}
	}

	reg := make([]string, 0, len(primaryKey))
	for key := range primaryKey {
		reg = append(reg, "'' "+EscapeIdentifier(key))
	}
	sort.Strings(reg)

	return strings.Join(reg, ",")
}

func getPrimaryKeyFakeEmptyValuesAssertion(primaryKey map[string]string, escapedTableName string) string {
	if len(primaryKey) == 1 {
		for key := range primaryKey {
			return escapedTableName + "." + EscapeIdentifier(key) + " IS NULL"
		}
	}

	reg := make([]string, 0, len(primaryKey))
	for key := range primaryKey {
		reg = append(reg, escapedTableName+"."+EscapeIdentifier(key)+" IS NULL")
	}
	sort.Strings(reg)

	return strings.Join(reg, " AND ")
}

func getPrimaryKeyWhereClause(primaryKey map[string]string, escapedTableName string) string {
	// Avoid any allocation if there is a single primary key
	if len(primaryKey) == 1 {
		for key, value := range primaryKey {
			if escapedTableName == "" {
				return EscapeIdentifier(key) + " = " + escapeStringValue(value)
			}

			return escapedTableName + "." + EscapeIdentifier(key) + " = " + escapeStringValue(value)
		}
	}

	reg := make([]string, 0, len(primaryKey))
	for key, value := range primaryKey {

		if escapedTableName == "" {
			reg = append(reg, EscapeIdentifier(key)+" = "+escapeStringValue(value))
		} else {
			reg = append(reg, escapedTableName+"."+EscapeIdentifier(key)+" = "+escapeStringValue(value))
		}
	}
	sort.Strings(reg)

	return strings.Join(reg[:], " AND ")
}

// Format based on type, value returned unescaped
func (d *PostgresDialect) normalizeValueType(value string, valueType reflect.Type) (string, error) {
	switch valueType.Kind() {
	case reflect.String:
		// replace unicode null character with empty string
		value = strings.ReplaceAll(value, "\u0000", "")
		return escapeStringValue(value), nil

	// BYTES in Postgres must be escaped, we receive a Vec<u8> from substreams
	case reflect.Slice:
		return escapeStringValue(value), nil

	case reflect.Bool:
		return fmt.Sprintf("'%s'", value), nil

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return value, nil

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return value, nil

	case reflect.Float32, reflect.Float64:
		return value, nil

	case reflect.Struct:
		if valueType == reflectTypeTime {
			if integerRegex.MatchString(value) {
				i, err := strconv.Atoi(value)
				if err != nil {
					return "", fmt.Errorf("could not convert %s to int: %w", value, err)
				}

				return escapeStringValue(time.Unix(int64(i), 0).Format(time.RFC3339)), nil
			}

			// It's a plain string, parse by dialect it and pass it to the database
			return d.ParseDatetimeNormalization(value), nil
		}

		return "", fmt.Errorf("unsupported struct type %s", valueType)
	default:
		// It's a column's type the schemaName parsing don't know how to represents as
		// a Go type. In that case, we pass it unmodified to the database engine. It
		// will be the responsibility of the one sending the data to correctly represent
		// it in the way accepted by the database.
		//
		// In most cases, it going to just work.
		return value, nil
	}
}

func (d PostgresDialect) GetTableColumns(db *sql.DB, schemaName, tableName string) ([]*sql.ColumnType, error) {
	query := fmt.Sprintf("SELECT * FROM %s.%s WHERE 1=0",
		EscapeIdentifier(schemaName),
		EscapeIdentifier(tableName))

	rows, err := db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("querying table structure: %w", err)
	}
	defer rows.Close()

	return rows.ColumnTypes()
}

func (d PostgresDialect) GetTablesInSchema(db *sql.DB, schemaName string) ([][2]string, error) {
	query := `
		SELECT table_schema, table_name
		FROM information_schema.tables
		WHERE table_type = 'BASE TABLE'
		AND table_schema = $1
		ORDER BY table_schema, table_name
	`

	rows, err := db.Query(query, schemaName)
	if err != nil {
		return nil, fmt.Errorf("querying tables: %w", err)
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

const postgresPrimaryKeyQuery = `
	SELECT kcu.column_name
	FROM information_schema.table_constraints tco
	JOIN information_schema.key_column_usage kcu
		ON kcu.constraint_name = tco.constraint_name
		AND kcu.constraint_schema = tco.constraint_schema
		AND kcu.table_name = tco.table_name
	WHERE tco.constraint_type = 'PRIMARY KEY'
		AND kcu.table_schema = %s
		AND kcu.table_name = %s
	ORDER BY kcu.ordinal_position`

func (d PostgresDialect) GetPrimaryKey(db *sql.DB, schemaName, tableName string) ([]string, error) {
	var query string
	var args []any

	if schemaName == "" {
		query = fmt.Sprintf(postgresPrimaryKeyQuery, "current_schema()", "$1")
		args = []any{tableName}
	} else {
		query = fmt.Sprintf(postgresPrimaryKeyQuery, "$1", "$2")
		args = []any{schemaName, tableName}
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
