package pbsubstreams

import (
	"fmt"
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
	tableMap := make(map[string][]*TableChange)
	for _, i := range x {
		tableMap[i.Table] = append(tableMap[i.Table], i)
	}

	result := make([]*TableChange, 0)
	for _, cs := range tableMap {
		switch len(cs) {
		case 0:
			continue
		case 1:
			result = append(result, cs[0])
		default:
			sort.Slice(cs, func(i, j int) bool {
				return cs[i].Ordinal < cs[j].Ordinal
			})

			newTableCreatedHere := cs[0].Operation == TableChange_CREATE

			for i := 0; i < len(cs)-1; i++ {
				prev := cs[0]
				next := cs[i+1]
				err := prev.Merge(next)
				if err != nil {
					return nil, err
				}
			}

			if newTableCreatedHere && cs[0].Operation == TableChange_DELETE {
				continue
			}

			result = append(result, cs[0])
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
		x.Operation = TableChange_DELETE
		x.Fields = next.Fields
	case TableChange_CREATE:
		if x.Operation != TableChange_DELETE {
			return fmt.Errorf("trying to create table when previous operation was not delete")
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
