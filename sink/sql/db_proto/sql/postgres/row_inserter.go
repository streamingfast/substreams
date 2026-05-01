package postgres

import (
	"database/sql"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"

	sql2 "github.com/streamingfast/substreams/sink/sql/db_proto/sql"
	"github.com/streamingfast/substreams/sink/sql/db_proto/sql/schema"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type RowInserter struct {
	insertQueries    map[string]string
	insertStatements map[string]*sql.Stmt
	logger           *zap.Logger
	database         *Database
}

func NewRowInserter(logger *zap.Logger) (*RowInserter, error) {
	logger = logger.Named("postgres inserter")

	return &RowInserter{
		logger: logger,
	}, nil
}

func (i *RowInserter) init(database *Database) error {
	tables := database.dialect.GetTables()
	insertStatements := map[string]*sql.Stmt{}
	insertQueries := map[string]string{}

	i.database = database

	for _, table := range tables {
		query, err := createInsertFromDescriptor(table, database.dialect)
		if err != nil {
			return fmt.Errorf("creating insert from descriptor for table %q: %w", table.Name, err)
		}
		insertQueries[table.Name] = query

		stmt, err := database.db.Prepare(query)
		if err != nil {
			return fmt.Errorf("preparing statement %q: %w", query, err)
		}
		insertStatements[table.Name] = stmt
	}

	insertQueries["_blocks_"] = fmt.Sprintf("INSERT INTO %s (number, hash, timestamp) VALUES ($1, $2, $3) RETURNING number", tableName(database.schema.Name, "_blocks_"))
	bs, err := database.db.Prepare(insertQueries["_blocks_"])
	if err != nil {
		return fmt.Errorf("preparing statement %q: %w", insertQueries["_blocks_"], err)
	}
	insertStatements["_blocks_"] = bs

	insertQueries["_cursor_"] = fmt.Sprintf("INSERT INTO %s (name, cursor) VALUES ($1, $2) ON CONFLICT (name) DO UPDATE SET cursor = $2", tableName(database.schema.Name, "_cursor_"))
	cs, err := database.db.Prepare(insertQueries["_cursor_"])
	if err != nil {
		return fmt.Errorf("preparing statement %q: %w", insertQueries["_cursor_"], err)
	}
	insertStatements["_cursor_"] = cs

	i.insertQueries = insertQueries
	i.insertStatements = insertStatements

	return nil
}

func createInsertFromDescriptor(table *schema.Table, dialect sql2.Dialect) (string, error) {
	tableName := dialect.FullTableName(table)
	fields := table.Columns

	var fieldNames []string
	var placeholders []string

	fieldCount := 0
	returningField := ""
	if table.PrimaryKey != nil {
		returningField = table.PrimaryKey.Name
	}

	fieldCount++
	fieldNames = append(fieldNames, sql2.DialectFieldBlockNumber)
	placeholders = append(placeholders, fmt.Sprintf("$%d", fieldCount))
	fieldCount++
	fieldNames = append(fieldNames, sql2.DialectFieldBlockTimestamp)
	placeholders = append(placeholders, fmt.Sprintf("$%d", fieldCount))

	if pk := table.PrimaryKey; pk != nil {
		fieldCount++
		returningField = pk.Name
		fieldNames = append(fieldNames, pk.Name)
		placeholders = append(placeholders, fmt.Sprintf("$%d", fieldCount))
	}

	if table.ChildOf != nil {
		fieldCount++
		fieldNames = append(fieldNames, table.ChildOf.ParentTableField)
		placeholders = append(placeholders, fmt.Sprintf("$%d", fieldCount))
	}

	for _, field := range fields {
		if field.Name == returningField {
			continue
		}
		if field.IsExtension {
			continue
		}
		if field.IsRepeated && field.Nested == nil {
			if field.IsMessage {
				continue
			}
		}
		fieldCount++
		fieldNames = append(fieldNames, field.QuotedName())
		placeholders = append(placeholders, fmt.Sprintf("$%d", fieldCount))
	}

	return fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
		tableName,
		strings.Join(fieldNames, ", "),
		strings.Join(placeholders, ", "),
	), nil

}

func (i *RowInserter) insert(table string, values []any, database *Database) error {
	i.logger.Debug("inserting row", zap.String("table", table), zap.Any("values", values))
	stmt := i.insertStatements[table]
	stmt = database.wrapInsertStatement(stmt)

	t := i.database.dialect.TableRegistry[table]

	fieldIndexOffset := 2
	if t != nil && t.ChildOf != nil {
		fieldIndexOffset = 3
	}

	for i, value := range values {

		var column *schema.Column
		fieldIndex := i - fieldIndexOffset

		if t != nil && fieldIndex >= 0 {
			column = t.Columns[fieldIndex]
		}

		switch v := value.(type) {
		case string:
			if column != nil && column.ConvertTo != nil && column.ConvertTo.Convertion != nil {
				if v == "" {
					values[i] = 0
				}
			}
		case uint64:
			values[i] = strconv.FormatUint(v, 10)
		case []uint8:
			if database.dialect.bytesEncoding.IsStringType() {
				encoded, err := database.dialect.bytesEncoding.EncodeBytes(v)
				if err != nil {
					return fmt.Errorf("failed to encode bytes: %v", err)
				}
				values[i] = encoded.(string)
				continue
			}
			values[i] = "'" + base64.StdEncoding.EncodeToString(v) + "'"
		case *timestamppb.Timestamp:
			values[i] = "'" + v.AsTime().Format(time.RFC3339) + "'"
		case []interface{}:
			var elements []string
			for _, elem := range v {
				elements = append(elements, ValueToString(elem, database.dialect.bytesEncoding))
			}
			values[i] = "{" + strings.Join(elements, ",") + "}"
		}
	}

	_, err := stmt.Exec(values...)
	if err != nil {
		insert := i.insertQueries[table]
		return fmt.Errorf("pg accumalator inserter: querying insert %q: %w", insert, err)
	}

	return nil
}

func (i *RowInserter) flush(database *Database) error {
	return nil
}
