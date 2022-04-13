package pbsubstreams

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/require"
)

func (x TableChanges) isEqual(other TableChanges) bool {
	if len(x) != len(other) {
		return false
	}

	sort.Slice(x, func(i, j int) bool {
		return x[i].Table < x[j].Table
	})

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

	sort.Slice(x.Fields, func(i, j int) bool {
		return x.Fields[i].Name < x.Fields[j].Name
	})

	sort.Slice(other.Fields, func(i, j int) bool {
		return x.Fields[i].Name < x.Fields[j].Name
	})

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
			Ordinal:   1,
			Pk:        "one",
			Operation: TableChange_CREATE,
			Fields: []*Field{
				{Name: "f1", OldValue: "", NewValue: "abc"},
				{Name: "f2", OldValue: "", NewValue: "10"},
			},
		},
		&TableChange{
			Table:     "table.1",
			Ordinal:   2,
			Pk:        "one",
			Operation: TableChange_UPDATE,
			Fields: []*Field{
				{Name: "f1", OldValue: "abc", NewValue: "pewpew"},
			},
		},
		&TableChange{
			Table:     "table.1",
			Ordinal:   3,
			Pk:        "one",
			Operation: TableChange_UPDATE,
			Fields: []*Field{
				{Name: "f1", OldValue: "pewpew", NewValue: "xyz"},
				{Name: "f2", OldValue: "10", NewValue: "23"},
			},
		},
		&TableChange{
			Table:     "table.2",
			Ordinal:   3,
			Pk:        "two",
			Operation: TableChange_UPDATE,
			Fields: []*Field{
				{Name: "g1", OldValue: "bar", NewValue: "foo"},
				{Name: "g2", OldValue: "0", NewValue: "1"},
			},
		},
		&TableChange{
			Table:     "table.2",
			Ordinal:   4,
			Pk:        "two",
			Operation: TableChange_DELETE,
			Fields:    []*Field{},
		},
		&TableChange{
			Table:     "table.3",
			Ordinal:   5,
			Pk:        "three",
			Operation: TableChange_CREATE,
			Fields: []*Field{
				{Name: "g1", OldValue: "", NewValue: "foo"},
				{Name: "g2", OldValue: "", NewValue: "1"},
			},
		},
		&TableChange{
			Table:     "table.3",
			Ordinal:   6,
			Pk:        "three",
			Operation: TableChange_DELETE,
			Fields:    []*Field{},
		},
		&TableChange{
			Table:     "table.4",
			Ordinal:   1,
			Pk:        "four.0",
			Operation: TableChange_CREATE,
			Fields: []*Field{
				{Name: "f1", OldValue: "", NewValue: "hello"},
				{Name: "f2", OldValue: "", NewValue: "42"},
			},
		},
		&TableChange{
			Table:     "table.4",
			Ordinal:   2,
			Pk:        "four.0",
			Operation: TableChange_UPDATE,
			Fields: []*Field{
				{Name: "f1", OldValue: "hello", NewValue: "goodbye"},
			},
		},
		&TableChange{
			Table:     "table.4",
			Ordinal:   3,
			Pk:        "four.1",
			Operation: TableChange_UPDATE,
			Fields: []*Field{
				{Name: "f1", OldValue: "wut", NewValue: "xyz"},
				{Name: "f2", OldValue: "10", NewValue: "23"},
			},
		},
		&TableChange{
			Table:     "table.4",
			Ordinal:   4,
			Pk:        "four.1",
			Operation: TableChange_UPDATE,
			Fields: []*Field{
				{Name: "f2", OldValue: "23", NewValue: "17"},
			},
		},
	)

	expected := []*TableChange{
		{
			Table:     "table.1",
			Ordinal:   3,
			Operation: TableChange_CREATE,
			Fields: []*Field{
				{Name: "f1", OldValue: "", NewValue: "xyz"},
				{Name: "f2", OldValue: "", NewValue: "23"},
			},
		},
		{
			Table:     "table.2",
			Ordinal:   4,
			Operation: TableChange_DELETE,
			Fields:    []*Field{},
		},
		{
			Table:     "table.4",
			Ordinal:   2,
			Pk:        "four.0",
			Operation: TableChange_CREATE,
			Fields: []*Field{
				{Name: "f1", OldValue: "", NewValue: "goodbye"},
				{Name: "f2", OldValue: "", NewValue: "42"},
			},
		},
		{
			Table:     "table.4",
			Ordinal:   4,
			Pk:        "four.1",
			Operation: TableChange_UPDATE,
			Fields: []*Field{
				{Name: "f1", OldValue: "wut", NewValue: "xyz"},
				{Name: "f2", OldValue: "10", NewValue: "17"},
			},
		},
	}

	changes, err := TableChanges(tableChanges).Merge()
	require.Nil(t, err)

	require.True(t, TableChanges(changes).isEqual(TableChanges(expected)))
}
