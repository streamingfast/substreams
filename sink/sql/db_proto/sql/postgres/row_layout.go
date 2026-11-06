package postgres

import (
	"github.com/streamingfast/substreams/sink/sql/db_proto/sql"
	"github.com/streamingfast/substreams/sink/sql/db_proto/sql/schema"
)

// rowValue is what one value of a row the walk produces for a table is stored in: a
// schema column, or for the block number, block timestamp and parent's key, which have
// none, only a column name.
type rowValue struct {
	name   string
	column *schema.Column
}

// rowLayout lists the values of a row the walk produces for the table, in order: the block
// number and timestamp, the primary key, the parent's key for a child table, then the
// table's other columns in schema order. The walk appends exactly one value per column.
func rowLayout(table *schema.Table) []rowValue {
	layout := []rowValue{{name: sql.DialectFieldBlockNumber}, {name: sql.DialectFieldBlockTimestamp}}

	if table.PrimaryKey != nil {
		for _, column := range table.Columns {
			if column.Name == table.PrimaryKey.Name {
				layout = append(layout, rowValue{name: column.Name, column: column})
				break
			}
		}
	}
	if table.ChildOf != nil {
		layout = append(layout, rowValue{name: table.ChildOf.ParentTableField})
	}

	for _, column := range table.Columns {
		if table.PrimaryKey != nil && column.Name == table.PrimaryKey.Name {
			continue
		}
		layout = append(layout, rowValue{name: column.Name, column: column})
	}

	return layout
}
