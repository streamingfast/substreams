package sql

import (
	"fmt"
	"sort"
	"strings"

	"github.com/streamingfast/substreams/sink/sql/db_proto/sql/schema"
	"go.uber.org/zap"
	"golang.org/x/exp/maps"
	"google.golang.org/protobuf/reflect/protoreflect"
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
	AppendInlineFieldValues(fieldValues []any, fd protoreflect.FieldDescriptor, fv protoreflect.Value, dm protoreflect.Message) ([]any, error)
}

type BaseDialect struct {
	CreateTableSql      map[string]string
	PrimaryKeySql       []*Constraint
	ForeignKeySql       []*Constraint
	UniqueConstraintSql []*Constraint
	// IndexSql holds the indexes the sink creates for itself rather than because the
	// schema asked for one. They are built in the same pass as the constraints, being the
	// same kind of expensive.
	IndexSql      []*Constraint
	TableRegistry map[string]*schema.Table
	Logger        *zap.Logger
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

// AddForeignKeyReferencing records the same statement along with the logical table it
// points at, which is what TableApplyOrder needs.
func (d *BaseDialect) AddForeignKeyReferencing(table string, referencedTable string, sql string) {
	d.ForeignKeySql = append(d.ForeignKeySql, &Constraint{Table: table, Sql: sql, ReferencedTable: referencedTable})
}

// TableApplyOrder returns every table ordered so that a referenced table always comes
// before the tables referencing it.
//
// Rows have to reach the server in that order whenever foreign keys are enforced, and
// the write paths group rows by table: a multi-row INSERT per table at flush, and one
// binary COPY per table in the buffer. Insertion order within a block is not enough,
// because the grouping loses it.
//
// Nesting depth is not the answer even though it looks like it: a table can point at a
// sibling it has no ancestry with, so this is a topological sort over the foreign keys
// themselves.
//
// A cycle has no valid order and is reported as an error rather than silently ordered
// wrong. A table referencing itself is not a cycle for these purposes — it constrains the
// order of rows within one table, which no table-level ordering can address — so those
// edges are skipped.
func (d *BaseDialect) TableApplyOrder() ([]string, error) {
	// The block table is referenced by every other one and referenced by nothing, so it
	// always leads.
	names := []string{DialectTableBlock}
	for name := range d.TableRegistry {
		names = append(names, name)
	}
	sort.Strings(names[1:])

	known := make(map[string]bool, len(names))
	for _, name := range names {
		known[name] = true
	}

	// referencedBy[x] lists the tables that must come after x; remaining[x] counts what x
	// still waits on.
	referencedBy := map[string][]string{}
	remaining := map[string]int{}
	for _, constraint := range d.ForeignKeySql {
		referenced := constraint.ReferencedTable
		if referenced == "" || referenced == constraint.Table || !known[referenced] || !known[constraint.Table] {
			continue
		}

		referencedBy[referenced] = append(referencedBy[referenced], constraint.Table)
		remaining[constraint.Table]++
	}

	var ready []string
	for _, name := range names {
		if remaining[name] == 0 {
			ready = append(ready, name)
		}
	}

	ordered := make([]string, 0, len(names))
	for len(ready) > 0 {
		name := ready[0]
		ready = ready[1:]
		ordered = append(ordered, name)

		for _, referencing := range referencedBy[name] {
			remaining[referencing]--
			if remaining[referencing] == 0 {
				ready = append(ready, referencing)
			}
		}
	}

	if len(ordered) != len(names) {
		var cycle []string
		for _, name := range names {
			if remaining[name] > 0 {
				cycle = append(cycle, name)
			}
		}

		return nil, fmt.Errorf("the foreign keys between %s form a cycle, so no order of tables can satisfy them; the schema needs one of those references dropped", strings.Join(cycle, ", "))
	}

	return ordered, nil
}

// TableApplyRanks is TableApplyOrder as a lookup, for sorting a set of tables that is not
// the whole schema. A table the dialect does not know about ranks last.
func (d *BaseDialect) TableApplyRanks() (map[string]int, error) {
	ordered, err := d.TableApplyOrder()
	if err != nil {
		return nil, err
	}

	ranks := make(map[string]int, len(ordered))
	for rank, name := range ordered {
		ranks[name] = rank
	}

	return ranks, nil
}

func (d *BaseDialect) AddIndexSql(table string, sql string) {
	d.IndexSql = append(d.IndexSql, &Constraint{Table: table, Sql: sql})
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
