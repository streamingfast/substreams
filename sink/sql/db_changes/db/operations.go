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

type UpdateOp int32

const (
	UpdateOpSet       UpdateOp = 0
	UpdateOpAdd       UpdateOp = 1
	UpdateOpMax       UpdateOp = 2
	UpdateOpMin       UpdateOp = 3
	UpdateOpSetIfNull UpdateOp = 4
)

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
	reversibleBlockNum *uint64
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

		if err := validateOpTransition(k, existing.UpdateOp, fd.UpdateOp); err != nil {
			return err
		}

		switch fd.UpdateOp {
		case UpdateOpSet:
			o.data[k] = fd

		case UpdateOpAdd:
			existingDec, err1 := parseDecimal(existing.Value)
			newDec, err2 := parseDecimal(fd.Value)
			if err1 == nil && err2 == nil {
				o.data[k] = FieldData{
					Value:    existingDec.Add(newDec).String(),
					UpdateOp: existing.UpdateOp,
				}
			} else {
				o.data[k] = fd
			}

		case UpdateOpMax:
			existingDec, err1 := parseDecimal(existing.Value)
			newDec, err2 := parseDecimal(fd.Value)
			if err1 == nil && err2 == nil {
				maxVal := existingDec
				if newDec.Cmp(existingDec.Rat) > 0 {
					maxVal = newDec
				}
				o.data[k] = FieldData{
					Value:    maxVal.String(),
					UpdateOp: existing.UpdateOp,
				}
			} else {
				o.data[k] = fd
			}

		case UpdateOpMin:
			existingDec, err1 := parseDecimal(existing.Value)
			newDec, err2 := parseDecimal(fd.Value)
			if err1 == nil && err2 == nil {
				minVal := existingDec
				if newDec.Cmp(existingDec.Rat) < 0 {
					minVal = newDec
				}
				o.data[k] = FieldData{
					Value:    minVal.String(),
					UpdateOp: existing.UpdateOp,
				}
			} else {
				o.data[k] = fd
			}

		case UpdateOpSetIfNull:
			continue
		}
	}
	return nil
}

func validateOpTransition(fieldName string, existing, incoming UpdateOp) error {
	if existing == UpdateOpSet {
		return nil
	}

	if incoming == UpdateOpSet {
		return nil
	}

	if existing == incoming {
		return nil
	}

	return fmt.Errorf(
		"invalid UpdateOp transition for field %q: cannot apply %s after %s (only %s \u2192 %s or SET \u2192 %s is allowed)",
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
	var d decimal
	_, ok := d.SetString(s)
	if !ok {
		return decimal{}, fmt.Errorf("invalid decimal: %s", s)
	}
	return d, nil
}

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

func primaryKeyToJSON(primaryKey map[string]string) string {
	m, err := json.Marshal(primaryKey)
	if err != nil {
		panic(err)
	}
	return string(m)
}

func jsonToPrimaryKey(in string) (map[string]string, error) {
	out := make(map[string]string)
	err := json.Unmarshal([]byte(in), &out)
	if err != nil {
		return nil, err
	}
	return out, nil
}
