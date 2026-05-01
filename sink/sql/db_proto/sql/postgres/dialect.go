package postgres

import (
	"encoding/hex"
	"fmt"
	"hash/fnv"
	"sort"
	"strings"

	"github.com/lib/pq"
	"github.com/streamingfast/substreams/sink/sql/bytes"
	sql2 "github.com/streamingfast/substreams/sink/sql/db_proto/sql"
	"github.com/streamingfast/substreams/sink/sql/db_proto/sql/schema"
	"go.uber.org/zap"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"
)

const postgresStaticSql = `
	CREATE SCHEMA IF NOT EXISTS "%s";

	CREATE TABLE IF NOT EXISTS "%s"._sink_info_ (
		schema_hash TEXT PRIMARY KEY
	);

	CREATE TABLE IF NOT EXISTS "%s"._cursor_ (
		name TEXT PRIMARY KEY,
		cursor TEXT NOT NULL
	);

	CREATE TABLE IF NOT EXISTS "%s"._blocks_ (
		number integer,
		hash TEXT NOT NULL,
		timestamp TIMESTAMP NOT NULL
	);
`

type DialectPostgres struct {
	*sql2.BaseDialect
	schemaName    string
	bytesEncoding bytes.Encoding
}

func NewDialectPostgres(schema *schema.Schema, bytesEncoding bytes.Encoding, logger *zap.Logger) (*DialectPostgres, error) {
	logger = logger.Named("postgres dialect")

	d := &DialectPostgres{
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

func (d *DialectPostgres) UseVersionField() bool {
	return false
}
func (d *DialectPostgres) UseDeletedField() bool {
	return false
}

func (d *DialectPostgres) init() error {
	d.AddPrimaryKeySql(sql2.DialectTableBlock, fmt.Sprintf("alter table %s.%s add constraint block_pk primary key (number);", d.schemaName, sql2.DialectTableBlock))
	return nil
}

func (d *DialectPostgres) createTable(table *schema.Table) error {
	var sb strings.Builder

	tableName := d.FullTableName(table)

	sb.WriteString(fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s (", tableName))

	sb.WriteString(fmt.Sprintf(" %s INTEGER NOT NULL,", sql2.DialectFieldBlockNumber))
	sb.WriteString(fmt.Sprintf(" %s TIMESTAMP NOT NULL,", sql2.DialectFieldBlockTimestamp))

	var primaryKeyFieldName string
	if table.PrimaryKey != nil {
		pk := table.PrimaryKey
		primaryKeyFieldName = pk.Name
		d.AddPrimaryKeySql(table.Name, fmt.Sprintf("alter table %s add constraint %s_pk primary key (%s);", tableName, table.Name, primaryKeyFieldName))
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

				foreignKey := &sql2.ForeignKey{
					Name:         "fk_" + table.ChildOf.ParentTable,
					Table:        tableName,
					Field:        table.ChildOf.ParentTableField,
					ForeignTable: d.FullTableName(parentTable),
					ForeignField: parentField.Name,
				}

				d.AddForeignKeySql(table.Name, foreignKey.String())

				fieldFound = true
				break
			}
		}
		if !fieldFound {
			return fmt.Errorf("field %q not found in table %q", table.ChildOf.ParentTableField, table.ChildOf.ParentTable)
		}
	}

	for _, f := range table.Columns {
		if f.Name == primaryKeyFieldName {
			continue
		}

		fieldQuotedName := f.QuotedName()

		switch {
		case f.IsRepeated:
		case f.Nested != nil:
			fmt.Println("found nested type")
		case f.IsMessage && !IsWellKnownType(f.FieldDescriptor):
			childTable, found := d.TableRegistry[f.Message]
			if !found {
				continue
			}
			if childTable.PrimaryKey == nil {
				continue
			}
			foreignKey := &sql2.ForeignKey{
				Name:         "fk_" + childTable.Name,
				Table:        tableName,
				Field:        fieldQuotedName,
				ForeignTable: d.FullTableName(childTable),
				ForeignField: childTable.PrimaryKey.Name,
			}
			d.AddForeignKeySql(table.Name, foreignKey.String())

		case f.ForeignKey != nil:
			foreignTable, found := d.TableRegistry[f.ForeignKey.Table]
			if !found {
				return fmt.Errorf("foreign table %q not found", f.ForeignKey.Table)
			}

			var foreignField *schema.Column
			for _, field := range foreignTable.Columns {
				if field.Name == f.ForeignKey.TableField {
					foreignField = field
					break
				}
			}
			if foreignField == nil {
				return fmt.Errorf("foreign field %q not found in table %q", f.ForeignKey.TableField, f.ForeignKey.Table)
			}

			foreignKey := &sql2.ForeignKey{
				Name:         "fk_" + f.Name,
				Table:        tableName,
				Field:        f.Name,
				ForeignTable: d.FullTableName(foreignTable),
				ForeignField: foreignField.Name,
			}
			d.AddForeignKeySql(table.Name, foreignKey.String())
		}
		fieldType := MapFieldType(f.FieldDescriptor, d.bytesEncoding, f)
		if f.IsUnique {
			d.AddUniqueConstraintSql(table.Name, fmt.Sprintf("alter table %s add constraint %s_%s_unique unique (%s);", tableName, table.Name, f.Name, fieldQuotedName))
		}

		sb.WriteString(fmt.Sprintf("%s %s", fieldQuotedName, fieldType))
		sb.WriteString(",")
	}

	temp := sb.String()
	temp = temp[:len(temp)-1]
	sb = strings.Builder{}
	sb.WriteString(temp)

	sb.WriteString(");\n")

	d.AddForeignKeySql(tableName, fmt.Sprintf("ALTER TABLE %s ADD CONSTRAINT fk_block FOREIGN KEY (%s) REFERENCES %s.%s(number) ON DELETE CASCADE", tableName, sql2.DialectFieldBlockNumber, d.schemaName, sql2.DialectTableBlock))
	d.AddCreateTableSql(table.Name, sb.String())

	return nil

}

func (d *DialectPostgres) FullTableName(table *schema.Table) string {
	return tableName(d.schemaName, table.Name)
}

func (d *DialectPostgres) AppendInlineFieldValues(fieldValues []any, fd protoreflect.FieldDescriptor, fv protoreflect.Value, dm *dynamicpb.Message) ([]any, error) {
	if fd.IsList() {
		list := fv.List()
		var jsonStrings []string
		for j := 0; j < list.Len(); j++ {
			fm := list.Get(j).Message().Interface().(*dynamicpb.Message)
			jsonBytes, err := protojson.Marshal(fm)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal protobuf message to JSON: %w", err)
			}
			jsonStrings = append(jsonStrings, string(jsonBytes))
		}
		fieldValues = append(fieldValues, pq.Array(jsonStrings))
	} else {
		fm := fv.Message().Interface().(*dynamicpb.Message)
		jsonBytes, err := protojson.Marshal(fm)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal protobuf message to JSON: %w", err)
		}
		fieldValues = append(fieldValues, string(jsonBytes))
	}
	return fieldValues, nil
}

func (d *DialectPostgres) SchemaHash() string {
	h := fnv.New64a()

	var buf []byte

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
