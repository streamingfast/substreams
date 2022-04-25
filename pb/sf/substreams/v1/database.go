package pbsubstreams

import (
	"fmt"
	"go.uber.org/zap"
	"sort"
)

type TableChanges []*TableChange

func (x *DatabaseChanges) Squash() error {
	changes, err := TableChanges(x.TableChanges).Merge()
	if err != nil {
		return err
	}

	x.TableChanges = changes
	return nil
}

func (x TableChanges) Merge() ([]*TableChange, error) {
	//group the table changes by key
	tableMap := make(map[string]map[string][]*TableChange)
	for _, i := range x {
		if _, ok := tableMap[i.Table]; !ok {
			tableMap[i.Table] = make(map[string][]*TableChange)
		}
		tableMap[i.Table][i.Pk] = append(tableMap[i.Table][i.Pk], i)
	}

	//merge each group
	result := make([]*TableChange, 0)
	for _, tableChanges := range tableMap {
		for _, tableChange := range tableChanges {
			switch len(tableChange) {
			case 0:
				continue
			case 1:
				result = append(result, tableChange[0])
			default:
				sort.Slice(tableChange, func(i, j int) bool {
					return tableChange[i].Ordinal < tableChange[j].Ordinal
				})

				currentTableChange := tableChange[0]
				createdHere := currentTableChange.Operation == TableChange_CREATE

				for i := 1; i <= len(tableChange)-1; i++ {
					next := tableChange[i]
					err := currentTableChange.Merge(next)
					if err != nil {
						return nil, err
					}
				}

				// row created and deleted in the same table change... we do nothing
				if createdHere && tableChange[0].Operation == TableChange_DELETE {
					continue
				}

				result = append(result, currentTableChange)
			}
		}
	}

	return result, nil
}

func (x *TableChange) Merge(next *TableChange) error {
	if x.Table != next.Table {
		return fmt.Errorf("table mismatch: %s != %s. merging only supported on same table", x.Table, next.Table)
	}

	if x.Ordinal >= next.Ordinal {
		return fmt.Errorf("non-increasing ordinal")
	}

	switch next.Operation {
	case TableChange_DELETE:
		x.Operation = next.Operation
		x.Fields = next.Fields
	case TableChange_CREATE:
		if x.Operation != TableChange_DELETE {
			zlog.Error("trying to create row when current operation is not delete, row already exists",
				zap.String("table", x.Table),
				zap.String("key", x.Pk),
				zap.String("last_operation", x.Operation.String()),
				zap.Reflect("fields", next.Fields))
			return fmt.Errorf("trying to create row when current operation is not delete, row already exists")
		}
		x.Operation = next.Operation
		x.Fields = next.Fields
	case TableChange_UPDATE:
		fieldValues := make(map[string]*Field)
		for _, oldField := range x.Fields {
			fieldValues[oldField.Name] = oldField
		}

		for _, newField := range next.Fields {
			oldField, ok := fieldValues[newField.Name]
			if !ok {
				fieldValues[newField.Name] = newField
				continue
			}

			if newField.OldValue != oldField.NewValue {
				return fmt.Errorf("update field mismatch: old value supposed to be %s, got %s", oldField.NewValue, newField.OldValue)
			}

			fieldValues[newField.Name] = &Field{
				Name:     newField.Name,
				NewValue: newField.NewValue,
				OldValue: oldField.OldValue,
			}
		}

		fields := make([]*Field, 0)
		for _, f := range fieldValues {
			fields = append(fields, f)
		}
		x.Fields = fields
	}

	x.Pk = next.Pk
	x.Ordinal = next.Ordinal
	x.BlockNum = next.BlockNum

	return nil
}
