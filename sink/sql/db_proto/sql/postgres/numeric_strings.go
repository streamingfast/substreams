package postgres

import (
	v1 "github.com/streamingfast/substreams/pb/sf/substreams/sink/sql/schema/v1"
	"github.com/streamingfast/substreams/sink/sql/db_proto/sql/schema"
)

// numericStringPositions lists where, in a row the walk produces for the table, the string
// fields converted to a NUMERIC column sit.
//
// The row is the block number and timestamp, the primary key, the parent's key for a child
// table, then the table's other columns in schema order: the walk appends exactly one value
// per column.
func numericStringPositions(table *schema.Table) []int {
	var positions []int

	position := 2
	if table.PrimaryKey != nil {
		position++
	}
	if table.ChildOf != nil {
		position++
	}

	for _, column := range table.Columns {
		if table.PrimaryKey != nil && column.Name == table.PrimaryKey.Name {
			if isNumericConversion(column) {
				positions = append(positions, 2)
			}
			continue
		}
		if isNumericConversion(column) {
			positions = append(positions, position)
		}
		position++
	}

	return positions
}

func isNumericConversion(column *schema.Column) bool {
	if column.ConvertTo == nil {
		return false
	}

	switch column.ConvertTo.Convertion.(type) {
	case *v1.StringConvertion_Int128, *v1.StringConvertion_Uint128,
		*v1.StringConvertion_Int256, *v1.StringConvertion_Uint256,
		*v1.StringConvertion_Decimal128, *v1.StringConvertion_Decimal256:
		return true
	}

	return false
}

// emptyNumericStringsAsZero stores an empty string bound for a NUMERIC column, or an empty
// element of one bound for a NUMERIC[] column, as 0, which is what binary COPY stores. The
// rendered write modes would send ” instead, which PostgreSQL rejects.
func emptyNumericStringsAsZero(value any) any {
	switch v := value.(type) {
	case string:
		if v == "" {
			return "0"
		}
	case []any:
		for i, element := range v {
			if element == "" {
				v[i] = "0"
			}
		}
	}

	return value
}
