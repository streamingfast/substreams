package clickhouse

import (
	"encoding/hex"
	"fmt"
	"hash/fnv"
	"sort"
	"strings"

	pbSchmema "github.com/streamingfast/substreams/pb/sf/substreams/sink/sql/schema/v1"
	"github.com/streamingfast/substreams/sink/sql/bytes"
	sql2 "github.com/streamingfast/substreams/sink/sql/db_proto/sql"
	"github.com/streamingfast/substreams/sink/sql/db_proto/sql/schema"
	"go.uber.org/zap"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"
)

const staticSqlCreatDatabase = `
	CREATE DATABASE IF NOT EXISTS %s;
`
const staticSqlCreateBlock = `
	CREATE TABLE IF NOT EXISTS %s._blocks_  (
		number    UInt64,
		hash      text,
		timestamp timestamp,
		version Int64,
		deleted bool

	)
	ENGINE = ReplacingMergeTree(version, deleted)
	PARTITION BY (toYYYYMM(timestamp))
	PRIMARY KEY (number)
	ORDER BY (number)
	SETTINGS
	    allow_experimental_replacing_merge_with_cleanup = 1;
`

const clickhouseTableOptionsErrorMsg = "schema annotation 'clickhouse_table_options' is required in table annotation 'option (schema.table) = { name: %q, ... }' , see: https://github.com/streamingfast/substreams/blob/develop/docs/references/sql/proto-annotations.md#clickhouse-specific-options for configuration details"

type DialectClickHouse struct {
	*sql2.BaseDialect
	schemaName    string
	bytesEncoding bytes.Encoding
}

func NewDialectClickHouse(schema *schema.Schema, bytesEncoding bytes.Encoding, logger *zap.Logger) (*DialectClickHouse, error) {
	d := &DialectClickHouse{
		BaseDialect:   sql2.NewBaseDialect(schema.TableRegistry, logger),
		schemaName:    schema.Name,
		bytesEncoding: bytesEncoding,
	}

	err := d.init()
	if err != nil {
		return nil, fmt.Errorf("initializing dialect: %w", err)
	}

	for _, table := range schema.TableRegistry {
		err := d.createTable(table)
		if err != nil {
			return nil, fmt.Errorf("handling table %q: %w", table.Name, err)
		}
	}

	return d, nil
}

func (d *DialectClickHouse) UseVersionField() bool {
	return true
}

func (d *DialectClickHouse) UseDeletedField() bool {
	return true
}

func (d *DialectClickHouse) init() error {
	return nil
}

func (d *DialectClickHouse) createTable(table *schema.Table) error {
	var sb strings.Builder

	tableName := d.FullTableName(table)

	sb.WriteString(fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s (", tableName))

	sb.WriteString(fmt.Sprintf(" %s UInt64 NOT NULL,", sql2.DialectFieldBlockNumber))
	sb.WriteString(fmt.Sprintf(" %s timestamp NOT NULL,", sql2.DialectFieldBlockTimestamp))
	sb.WriteString(fmt.Sprintf(" %s Int64 NOT NULL,", sql2.DialectFieldVersion))
	sb.WriteString(fmt.Sprintf(" %s bool NOT NULL,", sql2.DialectFieldDeleted))

	var primaryKeyFieldName string
	if table.PrimaryKey != nil {
		pk := table.PrimaryKey
		primaryKeyFieldName = pk.Name
		sb.WriteString(fmt.Sprintf("%s %s,", pk.Name, MapFieldType(pk.FieldDescriptor, d.bytesEncoding, table.Columns[pk.Index])))
	}

	if table.ChildOf != nil {
		parentTable, parentFound := d.TableRegistry[table.ChildOf.ParentTable]
		if !parentFound {
			return fmt.Errorf("parent table %q not found", table.ChildOf.ParentTable)
		}
		fieldFound := false
		for _, parentField := range parentTable.Columns {

			if parentField.Name == table.ChildOf.ParentTableField {
				sb.WriteString(fmt.Sprintf("%s %s NOT NULL,", parentField.Name, MapFieldType(parentField.FieldDescriptor, d.bytesEncoding, parentField)))
				fieldFound = true
				break
			}
		}
		if !fieldFound {
			return fmt.Errorf("field %q not found in table %q", table.ChildOf.ParentTableField, table.ChildOf.ParentTable)
		}
	}

	var buildNestedFields func(columns []*schema.Column) string
	buildNestedFields = func(columns []*schema.Column) string {
		var fields []string
		for _, nestedCol := range columns {
			if nestedCol.Nested != nil {
				// Handle nested within nested
				innerFields := buildNestedFields(nestedCol.Nested.Columns)
				fields = append(fields, fmt.Sprintf("%s Nested(%s)", nestedCol.Name, innerFields))
			} else {
				fieldType := MapFieldType(nestedCol.FieldDescriptor, d.bytesEncoding, nestedCol).String()
				fields = append(fields, fmt.Sprintf("%s %s", nestedCol.Name, fieldType))
			}
		}
		return strings.Join(fields, ", ")
	}

	var processColumn func(f *schema.Column, sb *strings.Builder)
	processColumn = func(f *schema.Column, sb *strings.Builder) {
		// Check if this column has nested structure
		if f.Nested != nil {
			// Use ClickHouse Nested() syntax instead of prefixing
			nestedFieldsStr := buildNestedFields(f.Nested.Columns)
			sb.WriteString(fmt.Sprintf("%s Nested(%s)", f.Name, nestedFieldsStr))
			sb.WriteString(",")
		} else {
			// Process regular column
			fieldType := MapFieldType(f.FieldDescriptor, d.bytesEncoding, f).String()
			sb.WriteString(fmt.Sprintf("%s %s", f.Name, fieldType))
			sb.WriteString(",")
		}
	}

	for _, f := range table.Columns {
		if f.Name == primaryKeyFieldName {
			continue
		}
		processColumn(f, &sb)
	}

	//removing the last comma since it is complicated to removing it before
	temp := sb.String()
	temp = temp[:len(temp)-1]
	sb = strings.Builder{}
	sb.WriteString(temp)

	replacingMergeTree := "ReplacingMergeTree(_version_, _deleted_)"

	primaryKey := ""
	if primaryKeyFieldName != "" {
		primaryKey = fmt.Sprintf("PRIMARY KEY (%s)", primaryKeyFieldName)
	}

	orderBy, err := orderByString(table)
	if err != nil {
		return fmt.Errorf("getting 'order by' string: %w", err)
	}

	partitionBy, err := partitionByString(table)
	if err != nil {
		return fmt.Errorf("getting 'partition by' string: %w", err)
	}

	// Add indexes if they exist
	indexes, err := indexString(table)
	if err != nil {
		return fmt.Errorf("getting 'index' string: %w", err)
	}

	sb.WriteString(fmt.Sprintf(" %s) ENGINE = %s %s %s %s", indexes, replacingMergeTree, primaryKey, partitionBy, orderBy))
	sb.WriteString(" SETTINGS\n")
	sb.WriteString(" allow_experimental_replacing_merge_with_cleanup = 1")
	sb.WriteString(";")

	d.AddCreateTableSql(table.Name, sb.String())

	return nil

}

func (d *DialectClickHouse) FullTableName(table *schema.Table) string {
	return tableName(d.schemaName, table.Name)
}

func (d *DialectClickHouse) AppendInlineFieldValues(fieldValues []any, fd protoreflect.FieldDescriptor, fv protoreflect.Value, dm *dynamicpb.Message) ([]any, error) {
	if fd.IsList() {
		// Handle as array of nested columns - flatten into multiple arrays
		list := fv.List()
		if list.Len() > 0 {
			firstMessage := list.Get(0).Message().Interface().(*dynamicpb.Message)
			nestedFields := firstMessage.Descriptor().Fields()

			// For each nested field, create an array of values from all list elements
			for j := 0; j < nestedFields.Len(); j++ {
				nestedFd := nestedFields.Get(j)
				var nestedValues []interface{}

				// Collect values for this nested field from all list elements
				for k := 0; k < list.Len(); k++ {
					fm := list.Get(k).Message().Interface().(*dynamicpb.Message)
					nestedValue := fm.Get(nestedFd)
					nestedValues = append(nestedValues, sql2.ScalarFieldValue(nestedFd, nestedValue))
				}

				fieldValues = append(fieldValues, nestedValues)
			}
		} else {
			// Empty list - need to get field count from descriptor
			// Get the message descriptor for this field type
			msgDesc := fd.Message()
			nestedFields := msgDesc.Fields()

			// Append empty arrays for each nested field
			for j := 0; j < nestedFields.Len(); j++ {
				fieldValues = append(fieldValues, []interface{}{})
			}
		}
	} else {
		// Handle as nested column - extract each field as an array
		fm := fv.Message().Interface().(*dynamicpb.Message)
		nestedFields := fm.Descriptor().Fields()
		for j := 0; j < nestedFields.Len(); j++ {
			nestedFd := nestedFields.Get(j)
			nestedValue := fm.Get(nestedFd)
			// Wrap the single value in an array (array of size 1)
			fieldValues = append(fieldValues, []interface{}{sql2.ScalarFieldValue(nestedFd, nestedValue)})
		}
	}
	return fieldValues, nil
}

func (d *DialectClickHouse) SchemaHash() string {
	h := fnv.New64a()

	var buf []byte

	// SchemaHash tableCreateStatements
	var sqls []string
	for _, sql := range d.CreateTableSql {
		sqls = append(sqls, sql)
	}

	sort.Strings(sqls)
	for _, sql := range sqls {
		buf = append(buf, []byte(sql)...)
	}

	var pk []string
	for _, constraint := range d.PrimaryKeySql {
		pk = append(pk, constraint.Sql)
	}
	sort.Strings(pk)
	for _, constraint := range pk {
		buf = append(buf, []byte(constraint)...)
	}

	var fk []string
	for _, constraint := range d.ForeignKeySql {
		fk = append(fk, constraint.Sql)
	}
	sort.Strings(fk)
	for _, constraint := range fk {
		buf = append(buf, []byte(constraint)...)
	}

	var uniques []string
	for _, constraint := range d.UniqueConstraintSql {
		uniques = append(uniques, constraint.Sql)
	}
	sort.Strings(uniques)
	for _, constraint := range uniques {
		buf = append(buf, []byte(constraint)...)
	}

	_, err := h.Write(buf)
	if err != nil {
		panic("unable to write to hash")
	}

	data := h.Sum(nil)
	return hex.EncodeToString(data)
}

func tableName(schemaName string, tableName string) string {
	return fmt.Sprintf("%s.%s", schemaName, tableName)
}

func orderByString(table *schema.Table) (string, error) {
	info := table.PbTableInfo.ClickhouseTableOptions
	if info == nil {
		return "", fmt.Errorf(clickhouseTableOptionsErrorMsg, table.Name)
	}

	if len(info.OrderByFields) == 0 {
		return "", fmt.Errorf("clickhouse table options for table %q don't have any 'order_by_fields'. Require at least 1", table.Name)
	}

	out := ""
	for i, field := range info.OrderByFields {
		w := wrapWithClickhouseFunction(field.Name, field.Function)
		if field.Descending {
			w += " desc"
		}
		out += w
		if i < len(info.OrderByFields)-1 {
			out += ", "
		}
	}

	return fmt.Sprintf("ORDER BY (%s)", out), nil
}

func partitionByString(table *schema.Table) (string, error) {
	info := table.PbTableInfo.ClickhouseTableOptions
	if info == nil {
		return "", fmt.Errorf(clickhouseTableOptionsErrorMsg, table.Name)
	}

	var parts []string

	// Check if any partition field is a function applied to _block_timestamp_
	hasBlockTimestampFunction := false
	for _, field := range info.PartitionFields {
		if field.Name == sql2.DialectFieldBlockTimestamp {
			hasBlockTimestampFunction = true
			break
		}
	}

	// Only include raw _block_timestamp_ if no function is applied to it
	if !hasBlockTimestampFunction {
		parts = append(parts, wrapWithClickhouseFunction(sql2.DialectFieldBlockTimestamp, pbSchmema.Function_toYYYYMM))
	}

	// Add all partition fields
	for _, field := range info.PartitionFields {
		w := wrapWithClickhouseFunction(field.Name, field.Function)
		parts = append(parts, w)
	}

	return fmt.Sprintf("PARTITION BY (%s)", strings.Join(parts, ", ")), nil
}

func wrapWithClickhouseFunction(fieldName string, function pbSchmema.Function) string {
	format := "%s"
	switch function {
	case pbSchmema.Function_unset:
	case pbSchmema.Function_toMonth:
		format = "toMonth(%s)"
	case pbSchmema.Function_toDate:
		format = "toDate(%s)"
	case pbSchmema.Function_toStartOfMonth:
	case pbSchmema.Function_toYear:
		format = "toYear(%s)"
	case pbSchmema.Function_toYYYYDD:
		format = "toYYYYMMDD(%s)"
	case pbSchmema.Function_toYYYYMM:
		format = "toYYYYMM(%s)"
	}
	return fmt.Sprintf(format, fieldName)
}

func indexString(table *schema.Table) (string, error) {
	indexes := ""
	if table.PbTableInfo != nil && table.PbTableInfo.ClickhouseTableOptions != nil {
		if len(table.PbTableInfo.ClickhouseTableOptions.IndexFields) > 0 {
			var indexStrings []string
			for _, indexField := range table.PbTableInfo.ClickhouseTableOptions.IndexFields {
				fieldName := indexField.FieldName
				if indexField.Function != pbSchmema.Function_unset {
					fieldName = fmt.Sprintf("%s(%s)", indexField.Function.String(), fieldName)
				}

				indexStr := fmt.Sprintf("INDEX %s %s TYPE %s GRANULARITY %d",
					indexField.Name,
					fieldName,
					indexField.Type.String(),
					indexField.Granularity)
				indexStrings = append(indexStrings, indexStr)
			}

			if len(indexStrings) > 0 {
				indexes = ", " + strings.Join(indexStrings, ", ")
			}
		}
	}
	return indexes, nil
}
