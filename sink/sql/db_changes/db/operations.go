package db

import (
	"encoding/json"
	"fmt"
	"math/big"
	"reflect"
	"regexp"
	"strings"
	"time"
)

type TypeGetter func(tableName string, columnName string) (reflect.Type, error)

type Queryable interface {
	query(d Dialect) (string, error)
}

type OperationType string

const (
	OperationTypeInsert OperationType = "INSERT"
	OperationTypeUpsert OperationType = "UPSERT"
	OperationTypeUpdate OperationType = "UPDATE"
	OperationTypeDelete OperationType = "DELETE"
)

// UpdateOp defines the operation to apply when updating a field on conflict
type UpdateOp int32

const (
	UpdateOpSet       UpdateOp = 0 // Direct assignment: col = value
	UpdateOpAdd       UpdateOp = 1 // Accumulate: col = COALESCE(col, 0) + value
	UpdateOpMax       UpdateOp = 2 // Maximum: col = GREATEST(COALESCE(col, 0), value)
	UpdateOpMin       UpdateOp = 3 // Minimum: col = LEAST(COALESCE(col, 0), value)
	UpdateOpSetIfNull UpdateOp = 4 // Set only if NULL: col = COALESCE(col, value)
)

// FieldData holds a field's value and its update operation
type FieldData struct {
	Value    string
	UpdateOp UpdateOp
}

type Operation struct {
	table              *TableInfo
	opType             OperationType
	primaryKey         map[string]string
	data               map[string]FieldData
	ordinal            uint64
	reversibleBlockNum *uint64 // nil if that block is known to be irreversible
}

func (o *Operation) String() string {
	return fmt.Sprintf("%s/%s (%s)", o.table.identifier, createRowUniqueID(o.primaryKey), strings.ToLower(string(o.opType)))
}

func (l *Loader) newInsertOperation(table *TableInfo, primaryKey map[string]string, data map[string]FieldData, ordinal uint64, reversibleBlockNum *uint64) *Operation {
	return &Operation{
		table:              table,
		opType:             OperationTypeInsert,
		primaryKey:         primaryKey,
		data:               data,
		ordinal:            ordinal,
		reversibleBlockNum: reversibleBlockNum,
	}
}

func (l *Loader) newUpsertOperation(table *TableInfo, primaryKey map[string]string, data map[string]FieldData, ordinal uint64, reversibleBlockNum *uint64) *Operation {
	return &Operation{
		table:              table,
		opType:             OperationTypeUpsert,
		primaryKey:         primaryKey,
		data:               data,
		ordinal:            ordinal,
		reversibleBlockNum: reversibleBlockNum,
	}
}

func (l *Loader) newUpdateOperation(table *TableInfo, primaryKey map[string]string, data map[string]FieldData, ordinal uint64, reversibleBlockNum *uint64) *Operation {
	return &Operation{
		table:              table,
		opType:             OperationTypeUpdate,
		primaryKey:         primaryKey,
		data:               data,
		ordinal:            ordinal,
		reversibleBlockNum: reversibleBlockNum,
	}
}

func (l *Loader) newDeleteOperation(table *TableInfo, primaryKey map[string]string, ordinal uint64, reversibleBlockNum *uint64) *Operation {
	return &Operation{
		table:              table,
		opType:             OperationTypeDelete,
		primaryKey:         primaryKey,
		ordinal:            ordinal,
		reversibleBlockNum: reversibleBlockNum,
	}
}

func (o *Operation) mergeData(newData map[string]FieldData) error {
	if o.opType == OperationTypeDelete {
		return fmt.Errorf("unable to merge data for a delete operation")
	}

	for k, fd := range newData {
		existing, exists := o.data[k]
		if !exists {
			o.data[k] = fd
			continue
		}

		// Validate transition based on strict rules (consistent with Rust library)
		// SET can be followed by any op, but non-SET ops can only be followed by same type
		if err := validateOpTransition(k, existing.UpdateOp, fd.UpdateOp); err != nil {
			return err
		}

		// Handle each incoming operation type
		switch fd.UpdateOp {
		case UpdateOpSet:
			// SET: latest value wins, overwrites any previous operation
			o.data[k] = fd

		case UpdateOpAdd:
			// ADD: accumulate values (valid after SET or ADD)
			existingDec, err1 := parseDecimal(existing.Value)
			newDec, err2 := parseDecimal(fd.Value)
			if err1 == nil && err2 == nil {
				o.data[k] = FieldData{
					Value:    existingDec.Add(newDec).String(),
					UpdateOp: existing.UpdateOp, // Keep existing op: SET stays SET, ADD stays ADD
				}
			} else {
				// Non-numeric: latest value wins
				o.data[k] = fd
			}

		case UpdateOpMax:
			// MAX: compute maximum (valid after SET or MAX)
			existingDec, err1 := parseDecimal(existing.Value)
			newDec, err2 := parseDecimal(fd.Value)
			if err1 == nil && err2 == nil {
				maxVal := existingDec
				if newDec.Cmp(existingDec.Rat) > 0 {
					maxVal = newDec
				}
				o.data[k] = FieldData{
					Value:    maxVal.String(),
					UpdateOp: existing.UpdateOp, // Keep existing op: SET stays SET, MAX stays MAX
				}
			} else {
				// Non-numeric: latest value wins
				o.data[k] = fd
			}

		case UpdateOpMin:
			// MIN: compute minimum (valid after SET or MIN)
			existingDec, err1 := parseDecimal(existing.Value)
			newDec, err2 := parseDecimal(fd.Value)
			if err1 == nil && err2 == nil {
				minVal := existingDec
				if newDec.Cmp(existingDec.Rat) < 0 {
					minVal = newDec
				}
				o.data[k] = FieldData{
					Value:    minVal.String(),
					UpdateOp: existing.UpdateOp, // Keep existing op: SET stays SET, MIN stays MIN
				}
			} else {
				// Non-numeric: latest value wins
				o.data[k] = fd
			}

		case UpdateOpSetIfNull:
			// SET_IF_NULL: keep existing value (first value wins)
			// Field already exists, so keep it and don't overwrite
			continue
		}
	}
	return nil
}

// validateOpTransition checks if the transition from existing to incoming op is valid.
// Returns an error for invalid transitions.
//
// Valid transitions:
//   - SET → any op: OK
//   - any op → SET: OK (SET always overwrites)
//   - ADD → ADD: OK (accumulates)
//   - MAX → MAX: OK (computes max)
//   - MIN → MIN: OK (computes min)
//   - SET_IF_NULL → SET_IF_NULL: OK (first value wins)
//
// All other transitions are invalid.
func validateOpTransition(fieldName string, existing, incoming UpdateOp) error {
	// SET can be followed by any operation
	if existing == UpdateOpSet {
		return nil
	}

	// Any operation can be followed by SET (SET overwrites)
	if incoming == UpdateOpSet {
		return nil
	}

	// Non-SET ops can only be followed by the same op type
	if existing == incoming {
		return nil
	}

	// Invalid transition
	return fmt.Errorf(
		"invalid UpdateOp transition for field %q: cannot apply %s after %s (only %s → %s or SET → %s is allowed)",
		fieldName,
		updateOpName(incoming),
		updateOpName(existing),
		updateOpName(existing),
		updateOpName(existing),
		updateOpName(incoming),
	)
}

func updateOpName(op UpdateOp) string {
	switch op {
	case UpdateOpSet:
		return "SET"
	case UpdateOpAdd:
		return "ADD"
	case UpdateOpMax:
		return "MAX"
	case UpdateOpMin:
		return "MIN"
	case UpdateOpSetIfNull:
		return "SET_IF_NULL"
	default:
		return fmt.Sprintf("UNKNOWN(%d)", op)
	}
}

func parseDecimal(s string) (decimal, error) {
	// Simple decimal parsing - just use big.Rat for precision
	var d decimal
	_, ok := d.SetString(s)
	if !ok {
		return decimal{}, fmt.Errorf("invalid decimal: %s", s)
	}
	return d, nil
}

// decimal is a simple wrapper around big.Rat for delta accumulation
type decimal struct {
	*big.Rat
}

func (d *decimal) SetString(s string) (*decimal, bool) {
	if d.Rat == nil {
		d.Rat = new(big.Rat)
	}
	_, ok := d.Rat.SetString(s)
	return d, ok
}

func (d decimal) Add(other decimal) decimal {
	result := new(big.Rat)
	result.Add(d.Rat, other.Rat)
	return decimal{result}
}

func (d decimal) Sub(other decimal) decimal {
	result := new(big.Rat)
	result.Sub(d.Rat, other.Rat)
	return decimal{result}
}

func (d decimal) Neg() decimal {
	result := new(big.Rat)
	result.Neg(d.Rat)
	return decimal{result}
}

func (d decimal) Sign() int {
	return d.Rat.Sign()
}

func (d decimal) String() string {
	return d.Rat.FloatString(18)
}

// mergeOperation merges another operation into this one, keeping the lowest ordinal
func (o *Operation) mergeOperation(otherData map[string]FieldData) error {
	if o.opType == OperationTypeDelete {
		return fmt.Errorf("unable to merge operation for a delete operation")
	}

	return o.mergeData(otherData)
}

var integerRegex = regexp.MustCompile(`^\d+$`)
var dateRegex = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
var reflectTypeTime = reflect.TypeOf(time.Time{})

func EscapeIdentifier(valueToEscape string) string {
	if strings.Contains(valueToEscape, `"`) {
		valueToEscape = strings.ReplaceAll(valueToEscape, `"`, `""`)
	}

	return `"` + valueToEscape + `"`
}

func escapeStringValue(valueToEscape string) string {
	if strings.Contains(valueToEscape, `'`) {
		valueToEscape = strings.ReplaceAll(valueToEscape, `'`, `''`)
	}

	return `'` + valueToEscape + `'`
}

// to store in an history table
func primaryKeyToJSON(primaryKey map[string]string) string {
	m, err := json.Marshal(primaryKey)
	if err != nil {
		panic(err) // should never happen with map[string]string
	}
	return string(m)
}

// to store in an history table
func jsonToPrimaryKey(in string) (map[string]string, error) {
	out := make(map[string]string)
	err := json.Unmarshal([]byte(in), &out)
	if err != nil {
		return nil, err
	}
	return out, nil
}
