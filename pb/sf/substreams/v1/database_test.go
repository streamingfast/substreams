package pbsubstreams

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/require"
)

func (x TableChanges) isEqual(t *testing.T, expected TableChanges) {
	//require.Equal(t, len(x), len(expected))

	sort.Slice(x, func(i, j int) bool {
		return x[i].Table < x[j].Table
	})

	for i, tc := range x {
		tc.isEqual(t, expected[i])
	}
}

func (x *TableChange) isEqual(t *testing.T, expected *TableChange) {
	sort.Slice(x.Fields, func(i, j int) bool {
		return x.Fields[i].Name < x.Fields[j].Name
	})

	sort.Slice(expected.Fields, func(i, j int) bool {
		return x.Fields[i].Name < x.Fields[j].Name
	})

	require.Equal(t, expected, x)
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
			Pk:        "one",
			Fields: []*Field{
				{Name: "f1", OldValue: "", NewValue: "xyz"},
				{Name: "f2", OldValue: "", NewValue: "23"},
			},
		},
		{
			Table:     "table.2",
			Ordinal:   4,
			Pk:        "two",
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

	TableChanges(changes).isEqual(t, expected)
}
