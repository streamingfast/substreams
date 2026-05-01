package postgres

import (
	"database/sql"
	"fmt"
	"strings"

	sql2 "github.com/streamingfast/substreams/sink/sql/db_proto/sql"
	"github.com/streamingfast/substreams/sink/sql/db_proto/sql/schema"
	"go.uber.org/zap"
)

type accumulator struct {
	query     string
	rowValues [][]string
}

type AccumulatorInserter struct {
	accumulators map[string]*accumulator
	cursorStmt   *sql.Stmt
	logger       *zap.Logger
}

func NewAccumulatorInserter(logger *zap.Logger) (*AccumulatorInserter, error) {
	logger = logger.Named("postgres inserter")

	return &AccumulatorInserter{
		logger: logger,
	}, nil
}

func (i *AccumulatorInserter) init(database *Database) error {
	tables := database.dialect.GetTables()
	accumulators := map[string]*accumulator{}

	for _, table := range tables {
		query, err := createInsertFromDescriptorAcc(table, database.dialect)
		if err != nil {
			return fmt.Errorf("creating insert from descriptor for table %q: %w", table.Name, err)
		}
		accumulators[table.Name] = &accumulator{
			query: query,
		}
	}
	accumulators["_blocks_"] = &accumulator{
		query: fmt.Sprintf("INSERT INTO %s (number, hash, timestamp) VALUES ", tableName(database.schema.Name, "_blocks_")),
	}

	cursorQuery := fmt.Sprintf("INSERT INTO %s (name, cursor) VALUES ($1, $2) ON CONFLICT (name) DO UPDATE SET cursor = $2", tableName(database.schema.Name, "_cursor_"))
	cs, err := database.db.Prepare(cursorQuery)
	if err != nil {
		return fmt.Errorf("preparing statement %q: %w", cursorQuery, err)
	}

	i.accumulators = accumulators
	i.cursorStmt = cs

	return nil
}

func createInsertFromDescriptorAcc(table *schema.Table, dialect sql2.Dialect) (string, error) {
	tableName := dialect.FullTableName(table)
	fields := table.Columns

	var fieldNames []string
	fieldNames = append(fieldNames, sql2.DialectFieldBlockNumber)
	fieldNames = append(fieldNames, sql2.DialectFieldBlockTimestamp)

	if pk := table.PrimaryKey; pk != nil {
		fieldNames = append(fieldNames, pk.Name)
	}

	if table.ChildOf != nil {
		fieldNames = append(fieldNames, table.ChildOf.ParentTableField)
	}

	for _, field := range fields {
		if table.PrimaryKey != nil && field.Name == table.PrimaryKey.Name {
			continue
		}

		if field.IsExtension {
			continue
		}
		if field.IsRepeated {
			if field.IsMessage {
				continue
			}
		}
		fieldNames = append(fieldNames, field.QuotedName())
	}

	return fmt.Sprintf("INSERT INTO %s (%s) VALUES ",
		tableName,
		strings.Join(fieldNames, ", "),
	), nil

}

func (i *AccumulatorInserter) insert(table string, values []any, database *Database) error {
	var v []string
	if table == "_cursor_" {
		stmt := database.wrapInsertStatement(i.cursorStmt)
		_, err := stmt.Exec(values...)
		if err != nil {
			return fmt.Errorf("executing insert: %w", err)
		}
		return nil
	}
	for _, value := range values {
		v = append(v, ValueToString(value, database.dialect.bytesEncoding))
	}
	accumulator := i.accumulators[table]
	if accumulator == nil {
		return fmt.Errorf("accumulator not found for table %q", table)
	}
	accumulator.rowValues = append(accumulator.rowValues, v)

	return nil
}

func (i *AccumulatorInserter) flush(database *Database) error {
	for _, acc := range i.accumulators {
		if len(acc.rowValues) == 0 {
			continue
		}
		var b strings.Builder
		b.WriteString(acc.query)
		for _, values := range acc.rowValues {
			b.WriteString("(")
			b.WriteString(strings.Join(values, ","))
			b.WriteString("),")
		}
		insert := strings.Trim(b.String(), ",")

		_, err := database.tx.Exec(insert)
		if err != nil {
			shortInsert := insert
			if len(insert) > 256 {
				shortInsert = insert[:256] + "..."
			}
			fmt.Println("insert query:", insert)
			return fmt.Errorf("executing insert %s: %w", shortInsert, err)
		}
		acc.rowValues = acc.rowValues[:0]
	}

	return nil
}
