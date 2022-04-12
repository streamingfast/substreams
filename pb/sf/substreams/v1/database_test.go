package pbsubstreams

import (
	"github.com/stretchr/testify/assert"
	"sort"
	"testing"
)

func (x TableChanges) isEqual(other TableChanges) bool {
	if len(x) != len(other) {
		return false
	}

	for i, tc := range x {
		if !tc.isEqual(other[i]) {
			return false
		}
	}

	return true
}

func (x *TableChange) isEqual(other *TableChange) bool {
	if x.Table != other.Table {
		return false
	}

	if x.BlockNum != other.BlockNum {
		return false
	}

	if x.Ordinal != other.Ordinal {
		return false
	}

	if x.Operation != other.Operation {
		return false
	}

	if len(x.Fields) != len(other.Fields) {
		return false
	}

	for i, f := range x.Fields {
		if !f.isEqual(other.Fields[i]) {
			return false
		}
	}

	return true
}

func (x *Field) isEqual(other *Field) bool {
	if x.Name != other.Name {
		return false
	}

	if x.OldValue != other.OldValue {
		return false
	}

	if x.NewValue != other.NewValue {
		return false
	}

	return true
}

func TestTableChanges_Merge(t *testing.T) {
	var tableChanges []*TableChange
	tableChanges = append(tableChanges,
		&TableChange{
			Table:     "table.1",
			BlockNum:  0,
			Ordinal:   1,
			Operation: TableChange_CREATE,
			Fields: []*Field{
				{Name: "f1", OldValue: "", NewValue: "abc"},
				{Name: "f2", OldValue: "", NewValue: "10"},
			},
		},
		&TableChange{
			Table:     "table.1",
			BlockNum:  0,
			Ordinal:   2,
			Operation: TableChange_UPDATE,
			Fields: []*Field{
				{Name: "f1", OldValue: "abc", NewValue: "pewpew"},
			},
		},
		&TableChange{
			Table:     "table.2",
			BlockNum:  0,
			Ordinal:   3,
			Operation: TableChange_UPDATE,
			Fields: []*Field{
				{Name: "g1", OldValue: "bar", NewValue: "foo"},
				{Name: "g2", OldValue: "0", NewValue: "1"},
			},
		},
		&TableChange{
			Table:     "table.1",
			BlockNum:  0,
			Ordinal:   3,
			Operation: TableChange_UPDATE,
			Fields: []*Field{
				{Name: "f1", OldValue: "pewpew", NewValue: "xyz"},
				{Name: "f2", OldValue: "10", NewValue: "23"},
			},
		},
		&TableChange{
			Table:     "table.2",
			BlockNum:  0,
			Ordinal:   4,
			Operation: TableChange_DELETE,
			Fields:    []*Field{},
		},
		&TableChange{
			Table:     "table.3",
			BlockNum:  0,
			Ordinal:   5,
			Operation: TableChange_CREATE,
			Fields: []*Field{
				{Name: "g1", OldValue: "", NewValue: "foo"},
				{Name: "g2", OldValue: "", NewValue: "1"},
			},
		},
		&TableChange{
			Table:     "table.3",
			BlockNum:  0,
			Ordinal:   6,
			Operation: TableChange_DELETE,
			Fields:    []*Field{},
		},
	)

	expected := []*TableChange{
		{
			Table:     "table.1",
			BlockNum:  0,
			Ordinal:   3,
			Operation: TableChange_CREATE,
			Fields: []*Field{
				{Name: "f1", OldValue: "", NewValue: "xyz"},
				{Name: "f2", OldValue: "", NewValue: "23"},
			},
		},
		{
			Table:     "table.2",
			BlockNum:  0,
			Ordinal:   4,
			Operation: TableChange_DELETE,
			Fields:    []*Field{},
		},
	}

	changes, err := TableChanges(tableChanges).Merge()

	sort.Slice(changes, func(i, j int) bool {
		return changes[i].Table < changes[j].Table
	})

	assert.Nil(t, err)

	assert.True(t, TableChanges(changes).isEqual(TableChanges(expected)))
}
