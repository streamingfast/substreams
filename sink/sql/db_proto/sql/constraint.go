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
}

func (f *ForeignKey) String() string {
	return fmt.Sprintf("ALTER TABLE %s ADD CONSTRAINT %s  FOREIGN KEY (%s) REFERENCES %s(%s)", f.Table, f.Name, f.Field, f.ForeignTable, f.ForeignField)
}
