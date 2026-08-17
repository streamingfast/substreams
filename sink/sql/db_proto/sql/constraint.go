package sql

import "fmt"

type ForeignKey struct {
	Name         string
	Table        string
	Field        string
	ForeignTable string
	ForeignField string
}

type Constraint struct {
	Table string
	Sql   string

	// ReferencedTable is the logical name of the table a foreign key points at, empty for
	// any other kind of constraint. It is what the apply order is computed from: the
	// SQL carries schema-qualified names, which is not what the registry is keyed by.
	ReferencedTable string
}

func (f *ForeignKey) String() string {
	return fmt.Sprintf("ALTER TABLE %s ADD CONSTRAINT %s  FOREIGN KEY (%s) REFERENCES %s(%s)", f.Table, f.Name, f.Field, f.ForeignTable, f.ForeignField)
}
