package db

import (
	"fmt"
	"maps"
	"regexp"
	"strconv"
	"strings"
)

// Row maps column names to values in FieldData's string form, nil for NULL.
type Row map[string]*string

func (r Row) clone() Row {
	out := make(Row, len(r))
	maps.Copy(out, r)
	return out
}

type rowOutcome int

const (
	rowUnchanged rowOutcome = iota
	rowWritten
	rowDeleted
)

// evaluateOperation applies an operation to the current state of its row (nil when it does not
// exist) and returns the row's new state:
//
//   - insert creates the row, and fails if it exists;
//   - upsert creates the row, or applies each field's UpdateOp to it;
//   - update applies each field's UpdateOp to the row, if it exists;
//   - delete deletes the row, if it exists.
//
// On an existing row, with NULL counted as 0: SET is v, ADD is cur+v, MAX is max(cur, v), MIN is
// min(cur, v), SET_IF_NULL is cur unless NULL. An upsert's v is first converted to the column's
// type (-0.5 is -1 in an integer column), an update's is used as given. A new row takes its values
// as given. These are the SQL sink's semantics on every database; the parity test checks them
// against PostgreSQL.
func evaluateOperation(op *Operation, current Row) (Row, rowOutcome, error) {
	switch op.opType {
	case OperationTypeInsert:
		if current != nil {
			return nil, rowUnchanged, fmt.Errorf("inserting row %s: a row with this primary key already exists", op)
		}
		row, err := newRow(op)
		return row, rowWritten, err

	case OperationTypeUpsert:
		if current == nil {
			row, err := newRow(op)
			return row, rowWritten, err
		}
		row, err := applyFields(op, current)
		return row, rowWritten, err

	case OperationTypeUpdate:
		if current == nil {
			return nil, rowUnchanged, nil
		}
		row, err := applyFields(op, current)
		return row, rowWritten, err

	case OperationTypeDelete:
		if current == nil {
			return nil, rowUnchanged, nil
		}
		return nil, rowDeleted, nil

	default:
		return nil, rowUnchanged, fmt.Errorf("unknown operation type %q", op.opType)
	}
}

func newRow(op *Operation) (Row, error) {
	row := make(Row, len(op.data)+len(op.primaryKey))
	for name, value := range op.primaryKey {
		row[name] = &value
	}
	for name, field := range op.data {
		column, found := op.table.columnsByName[name]
		if !found {
			return nil, unknownColumnError(op, name)
		}
		value := normalizeNumeric(column, field.Value)
		row[name] = &value
	}
	return row, nil
}

// normalizeNumeric writes a number for a numeric column's type ("1.0" into an integer column, say).
// Other values, and values of non-numeric columns, are left as they are.
func normalizeNumeric(column *ColumnInfo, value string) string {
	if kind, _ := numericColumnKind(column.databaseTypeName); kind == numericOther {
		return value
	}
	n, err := parseDecimal(strings.TrimSpace(value))
	if err != nil {
		return value
	}
	return formatForColumn(n, column)
}

func applyFields(op *Operation, current Row) (Row, error) {
	row := current.clone()
	for name, field := range op.data {
		column, found := op.table.columnsByName[name]
		if !found {
			return nil, unknownColumnError(op, name)
		}

		operand := field.Value
		if op.opType == OperationTypeUpsert {
			operand = normalizeNumeric(column, field.Value)
		}
		value, err := applyUpdateOp(column, field.UpdateOp, operand, current[name])
		if err != nil {
			return nil, fmt.Errorf("row %s, column %q: %w", op, name, err)
		}
		row[name] = value
	}
	return row, nil
}

func unknownColumnError(op *Operation, name string) error {
	return fmt.Errorf("row %s: unknown column %q", op, name)
}

func applyUpdateOp(column *ColumnInfo, updateOp UpdateOp, operand string, current *string) (*string, error) {
	switch updateOp {
	case UpdateOpSet:
		value := normalizeNumeric(column, operand)
		return &value, nil

	case UpdateOpSetIfNull:
		if current != nil {
			return current, nil
		}
		value := normalizeNumeric(column, operand)
		return &value, nil

	case UpdateOpAdd, UpdateOpMax, UpdateOpMin:
		cur := "0"
		if current != nil {
			cur = *current
		}
		curNum, err := parseDecimal(strings.TrimSpace(cur))
		if err != nil {
			return nil, fmt.Errorf("current value %q is not a number: %w", cur, err)
		}
		newNum, err := parseDecimal(strings.TrimSpace(operand))
		if err != nil {
			return nil, fmt.Errorf("%s value %q is not a number: %w", updateOpName(updateOp), operand, err)
		}

		result := curNum
		switch updateOp {
		case UpdateOpAdd:
			result = curNum.Add(newNum)
		case UpdateOpMax:
			if newNum.Cmp(curNum) > 0 {
				result = newNum
			}
		case UpdateOpMin:
			if newNum.Cmp(curNum) < 0 {
				result = newNum
			}
		}

		out := formatForColumn(result, column)
		return &out, nil

	default:
		return nil, fmt.Errorf("unknown UpdateOp %d", updateOp)
	}
}

// formatForColumn writes a number for the column's type: integers rounded half away from zero,
// decimals at the column's scale, floats in their shortest form, anything else (a text column)
// with the number's own scale.
func formatForColumn(n decimal, column *ColumnInfo) string {
	switch kind, scale := numericColumnKind(column.databaseTypeName); kind {
	case numericInteger:
		return n.Round(0).StringFixed(0)
	case numericDecimal:
		return n.StringFixed(int32(scale))
	case numericFloat:
		return strconv.FormatFloat(n.InexactFloat64(), 'g', -1, 64)
	default:
		return n.String()
	}
}

type numericKind int

const (
	numericOther numericKind = iota
	numericInteger
	numericDecimal
	numericFloat
)

var (
	integerTypeRegex  = regexp.MustCompile(`^U?Int(8|16|32|64|128|256)$`)
	decimalTypeRegex  = regexp.MustCompile(`^Decimal\(\s*\d+\s*,\s*(\d+)\s*\)$`)
	decimalNTypeRegex = regexp.MustCompile(`^Decimal(32|64|128|256)\(\s*(\d+)\s*\)$`)
)

// numericColumnKind classifies a ClickHouse column type, with its scale for decimals.
func numericColumnKind(databaseTypeName string) (numericKind, int) {
	t := unwrapColumnType(databaseTypeName)
	switch {
	case integerTypeRegex.MatchString(t):
		return numericInteger, 0
	case t == "Float32" || t == "Float64":
		return numericFloat, 0
	}
	if m := decimalTypeRegex.FindStringSubmatch(t); m != nil {
		scale, _ := strconv.Atoi(m[1])
		return numericDecimal, scale
	}
	if m := decimalNTypeRegex.FindStringSubmatch(t); m != nil {
		scale, _ := strconv.Atoi(m[2])
		return numericDecimal, scale
	}
	return numericOther, 0
}

// unwrapColumnType removes the Nullable(...) and LowCardinality(...) wrappers of a ClickHouse
// column type.
func unwrapColumnType(t string) string {
	for {
		trimmed := strings.TrimSpace(t)
		switch {
		case strings.HasPrefix(trimmed, "Nullable(") && strings.HasSuffix(trimmed, ")"):
			t = trimmed[len("Nullable(") : len(trimmed)-1]
		case strings.HasPrefix(trimmed, "LowCardinality(") && strings.HasSuffix(trimmed, ")"):
			t = trimmed[len("LowCardinality(") : len(trimmed)-1]
		default:
			return trimmed
		}
	}
}
