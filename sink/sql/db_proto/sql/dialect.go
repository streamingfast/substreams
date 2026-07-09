package sql

import (
	"github.com/streamingfast/substreams/sink/sql/db_proto/sql/schema"
	"go.uber.org/zap"
	"golang.org/x/exp/maps"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"
)

const DialectTableBlock = "_blocks_"
const DialectTableCursor = "_cursors_"

const DialectFieldBlockNumber = "_block_number_"
const DialectFieldBlockTimestamp = "_block_timestamp_"
const DialectFieldVersion = "_version_"
const DialectFieldDeleted = "_deleted_"

type Dialect interface {
	SchemaHash() string
	FullTableName(table *schema.Table) string
	GetTable(table string) *schema.Table
	GetTables() []*schema.Table
	UseVersionField() bool
	UseDeletedField() bool
	AppendInlineFieldValues(fieldValues []any, fd protoreflect.FieldDescriptor, fv protoreflect.Value, dm *dynamicpb.Message) ([]any, error)
}

type BaseDialect struct {
	CreateTableSql      map[string]string
	PrimaryKeySql       []*Constraint
	ForeignKeySql       []*Constraint
	UniqueConstraintSql []*Constraint
	TableRegistry       map[string]*schema.Table
	Logger              *zap.Logger
}

func NewBaseDialect(registry map[string]*schema.Table, logger *zap.Logger) *BaseDialect {
	return &BaseDialect{
		CreateTableSql: make(map[string]string),
		TableRegistry:  registry,
		Logger:         logger,
	}
}

func (d *BaseDialect) AddCreateTableSql(table string, sql string) {
	d.CreateTableSql[table] = sql
}

func (d *BaseDialect) GetCreateTableSql(table string) string {
	return d.CreateTableSql[table]
}

func (d *BaseDialect) AddPrimaryKeySql(table string, sql string) {
	d.PrimaryKeySql = append(d.PrimaryKeySql, &Constraint{Table: table, Sql: sql})
}

func (d *BaseDialect) AddForeignKeySql(table string, sql string) {
	d.ForeignKeySql = append(d.ForeignKeySql, &Constraint{Table: table, Sql: sql})
}

func (d *BaseDialect) AddUniqueConstraintSql(table string, sql string) {
	d.UniqueConstraintSql = append(d.UniqueConstraintSql, &Constraint{Table: table, Sql: sql})
}

func (d *BaseDialect) GetTable(table string) *schema.Table {
	return d.TableRegistry[table]
}

func (d *BaseDialect) GetTables() []*schema.Table {
	return maps.Values(d.TableRegistry)
}
