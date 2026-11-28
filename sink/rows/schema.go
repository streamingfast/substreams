package rows

import (
	"google.golang.org/protobuf/reflect/protoreflect"
)

// TableID is the dense index of a [Table] in [Schema.Tables].
type TableID int

// ColumnID is the index of a [Column] in [Table.Columns], which is also its index in [Row.Values].
type ColumnID int

//go:generate go-enum -f=$GOFILE --marshal --names

// PKKind tells how a table is keyed.
//
// ENUM(none, single, composite)
type PKKind uint8

// PrimaryKey is a sum type: the common single-column key needs no slice and no loop.
type PrimaryKey struct {
	Kind PKKind
	// Single is the key column when Kind is [PKKindSingle].
	Single ColumnID
	// Composite holds the key columns, in key order, when Kind is [PKKindComposite].
	Composite []ColumnID
}

// Columns returns the key columns in key order. It allocates for a single-column key, use Kind and Single on hot paths.
func (k PrimaryKey) Columns() []ColumnID {
	switch k.Kind {
	case PKKindSingle:
		return []ColumnID{k.Single}
	case PKKindComposite:
		return k.Composite
	default:
		return nil
	}
}

// Len is the number of key columns.
func (k PrimaryKey) Len() int {
	switch k.Kind {
	case PKKindSingle:
		return 1
	case PKKindComposite:
		return len(k.Composite)
	default:
		return 0
	}
}

// At returns the i-th key column.
func (k PrimaryKey) At(i int) ColumnID {
	if k.Kind == PKKindSingle {
		return k.Single
	}
	return k.Composite[i]
}

// ChildOf is the `child_of: "<table> on <column>"` annotation of a table.
type ChildOf struct {
	ParentTable string
	ParentField string
}

// ForeignKey is the `foreign_key: "<table> on <column>"` annotation of a column.
type ForeignKey struct {
	Table string
	Field string
}

// Column is one column of a [Table].
type Column struct {
	ID    ColumnID
	Name  string // (field).name override, or the protobuf field name
	Field protoreflect.FieldDescriptor
	Type  LogicalType
	// Optional is true when the value can be absent: proto3 optional, an unset message, a oneof member.
	Optional bool
	PK       bool
	Unique   bool
	// HasFK tells whether FK is set.
	HasFK bool
	FK    ForeignKey
}

// Table is one table of the schema: a protobuf message carrying a table annotation, or any message when
// annotations are not required.
type Table struct {
	ID      TableID
	Name    string
	Message protoreflect.MessageDescriptor
	Columns []Column
	PK      PrimaryKey
	// HasChildOf tells whether ChildOf is set.
	HasChildOf bool
	ChildOf    ChildOf
	// Children are the tables directly reachable under this one, through pass-through messages too.
	Children []TableID
}

// Column returns the column called name, or nil.
func (t *Table) Column(name string) *Column {
	for i := range t.Columns {
		if t.Columns[i].Name == name {
			return &t.Columns[i]
		}
	}
	return nil
}

// Schema is the set of tables rows refer to. Once resolved it is immutable, except for [Schema.Merge] on
// producers that discover tables at runtime.
type Schema struct {
	// Root is the output message descriptor the schema was resolved from, nil for schemas built by hand or loaded from a store.
	Root   protoreflect.MessageDescriptor
	Tables []Table

	version uint64

	byName map[string]TableID
	// rootPlan and maxColumns drive the walk of the schema producer, nil/0 for other producers.
	rootPlan   *msgPlan
	maxColumns int
}

// Table returns the table called name, or nil.
func (s *Schema) Table(name string) *Table {
	if id, found := s.byName[name]; found {
		return &s.Tables[id]
	}
	return nil
}

// Version increments every time a table or a column is added to a discovering producer's schema.
func (s *Schema) Version() uint64 { return s.version }

func (s *Schema) index() {
	s.byName = make(map[string]TableID, len(s.Tables))
	for i := range s.Tables {
		s.byName[s.Tables[i].Name] = s.Tables[i].ID
	}
}
