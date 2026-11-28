package rows

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	pbschema "github.com/streamingfast/substreams/pb/sf/substreams/sink/schema/v1"
)

// Marshal encodes the schema as a protobuf `sf.substreams.sink.schema.v1.Schema`. Descriptors are not serialized: a loaded
// schema carries names, kinds, bits, scale and keys, which is what a consumer needs to type its columns.
func (s *Schema) Marshal() ([]byte, error) {
	out := &pbschema.Schema{Version: s.version}
	for i := range s.Tables {
		t := &s.Tables[i]

		pt := &pbschema.Schema_Table{Name: t.Name, PrimaryKey: &pbschema.Schema_PrimaryKey{}}
		if t.Message != nil {
			pt.Message = string(t.Message.FullName())
		}
		switch t.PK.Kind {
		case PKKindSingle:
			pt.PrimaryKey.Key = &pbschema.Schema_PrimaryKey_Single{Single: uint32(t.PK.Single)}
		case PKKindComposite:
			columns := make([]uint32, len(t.PK.Composite))
			for k, c := range t.PK.Composite {
				columns[k] = uint32(c)
			}
			pt.PrimaryKey.Key = &pbschema.Schema_PrimaryKey_Composite{Composite: &pbschema.Schema_CompositeKey{Columns: columns}}
		}
		if t.HasChildOf {
			pt.ChildOf = &pbschema.Schema_ChildOf{ParentTable: t.ChildOf.ParentTable, ParentField: t.ChildOf.ParentField}
		}

		for k := range t.Columns {
			c := &t.Columns[k]
			pc := &pbschema.Schema_Column{Name: c.Name, Optional: c.Optional, Unique: c.Unique, Type: marshalType(c.Type)}
			if c.HasFK {
				pc.ForeignKey = &pbschema.Schema_ForeignKey{Table: c.FK.Table, Field: c.FK.Field}
			}
			pt.Columns = append(pt.Columns, pc)
		}
		out.Tables = append(out.Tables, pt)
	}

	return out.MarshalVT()
}

func marshalType(t LogicalType) *pbschema.Schema_LogicalType {
	out := &pbschema.Schema_LogicalType{Kind: t.Kind.String(), Bits: uint32(t.Bits), Signed: t.Signed, Scale: t.Scale}
	if t.Kind == KindList {
		out.Elem = t.Elem.String()
	}
	if t.Enum != nil {
		out.Enum = string(t.Enum.FullName())
	}
	if t.Struct != nil {
		out.Struct = string(t.Struct.FullName())
	}
	return out
}

// UnmarshalSchema decodes a schema saved by [Schema.Marshal]. The schema has no descriptors and cannot drive the
// schema producer, it types the columns of producers that discover their tables.
func UnmarshalSchema(data []byte) (*Schema, error) {
	in := &pbschema.Schema{}
	if err := in.UnmarshalVT(data); err != nil {
		return nil, fmt.Errorf("decode schema: %w", err)
	}

	s := &Schema{version: in.Version, Tables: make([]Table, len(in.Tables))}
	for i, pt := range in.Tables {
		t := Table{ID: TableID(i), Name: pt.Name, Columns: make([]Column, len(pt.Columns))}
		for _, known := range s.Tables[:i] {
			if known.Name == pt.Name {
				return nil, fmt.Errorf("duplicate table %q", pt.Name)
			}
		}

		switch key := pt.GetPrimaryKey().GetKey().(type) {
		case *pbschema.Schema_PrimaryKey_Single:
			t.PK = PrimaryKey{Kind: PKKindSingle, Single: ColumnID(key.Single)}
		case *pbschema.Schema_PrimaryKey_Composite:
			t.PK.Kind = PKKindComposite
			for _, c := range key.Composite.GetColumns() {
				t.PK.Composite = append(t.PK.Composite, ColumnID(c))
			}
			if len(t.PK.Composite) < 2 {
				return nil, fmt.Errorf("table %q: a composite primary key needs at least 2 columns, got %d", pt.Name, len(t.PK.Composite))
			}
		}
		if pt.ChildOf != nil {
			t.HasChildOf, t.ChildOf = true, ChildOf{ParentTable: pt.ChildOf.ParentTable, ParentField: pt.ChildOf.ParentField}
		}

		for k, pc := range pt.Columns {
			typ, err := unmarshalType(pc.Type)
			if err != nil {
				return nil, fmt.Errorf("table %q column %q: %w", pt.Name, pc.Name, err)
			}

			for _, known := range t.Columns[:k] {
				if known.Name == pc.Name {
					return nil, fmt.Errorf("table %q: duplicate column %q", pt.Name, pc.Name)
				}
			}

			c := Column{ID: ColumnID(k), Name: pc.Name, Type: typ, Optional: pc.Optional, Unique: pc.Unique}
			if pc.ForeignKey != nil {
				c.HasFK, c.FK = true, ForeignKey{Table: pc.ForeignKey.Table, Field: pc.ForeignKey.Field}
			}
			t.Columns[k] = c
		}

		for _, id := range t.PK.Columns() {
			if int(id) >= len(t.Columns) {
				return nil, fmt.Errorf("table %q: primary key column %d out of range", t.Name, id)
			}
			t.Columns[id].PK = true
		}
		s.Tables[i] = t
		s.maxColumns = max(s.maxColumns, len(t.Columns))
	}

	s.index()
	return s, nil
}

func unmarshalType(in *pbschema.Schema_LogicalType) (LogicalType, error) {
	kind, err := ParseKind(in.GetKind())
	if err != nil {
		return LogicalType{}, err
	}

	out := LogicalType{Kind: kind, Bits: uint16(in.Bits), Signed: in.Signed, Scale: in.Scale}
	if kind == KindList {
		if out.Elem, err = ParseKind(in.Elem); err != nil {
			return LogicalType{}, fmt.Errorf("list element: %w", err)
		}
	}
	return out, nil
}

// sameType compares two logical types ignoring descriptors, which a loaded schema does not have.
func sameType(a, b LogicalType) bool {
	return a.Kind == b.Kind && a.Elem == b.Elem && a.Bits == b.Bits && a.Signed == b.Signed && a.Scale == b.Scale
}

// Compatible tells whether s is prev with tables and columns appended: nothing removed, reordered or retyped. It is the
// rule a discovered schema follows, ids stay valid because they only grow.
func (s *Schema) Compatible(prev *Schema) error {
	for i := range prev.Tables {
		old := &prev.Tables[i]

		t := s.Table(old.Name)
		if t == nil {
			return fmt.Errorf("table %q was removed", old.Name)
		}
		if len(t.Columns) < len(old.Columns) {
			return fmt.Errorf("table %q lost columns: %d before, %d now", old.Name, len(old.Columns), len(t.Columns))
		}

		for k := range old.Columns {
			a, b := &old.Columns[k], &t.Columns[k]
			if a.Name != b.Name {
				return fmt.Errorf("table %q column %d was %q and is now %q (columns are append-only)", old.Name, k, a.Name, b.Name)
			}
			if !sameType(a.Type, b.Type) {
				return fmt.Errorf("table %q column %q changed type from %s to %s", old.Name, a.Name, a.Type, b.Type)
			}
			if a.Optional != b.Optional || a.Unique != b.Unique || a.HasFK != b.HasFK || a.FK != b.FK {
				return fmt.Errorf("table %q column %q changed its optional, unique or foreign key flags", old.Name, a.Name)
			}
		}

		if old.PK.Kind != t.PK.Kind || old.PK.Single != t.PK.Single || !slices.Equal(old.PK.Composite, t.PK.Composite) {
			return fmt.Errorf("table %q primary key changed", old.Name)
		}
		if old.HasChildOf != t.HasChildOf || old.ChildOf != t.ChildOf {
			return fmt.Errorf("table %q child_of changed", old.Name)
		}
	}
	return nil
}

// Merge adds the tables and columns of next that s does not have yet, keeping every existing id, and bumps the version
// when something was added. next must extend s, see [Schema.Compatible], otherwise nothing is changed.
//
// Appending to Tables can move it in memory: a *Table obtained before a Merge that returned changed must be fetched again.
func (s *Schema) Merge(next *Schema) (changed bool, err error) {
	if err := next.Compatible(s); err != nil {
		return false, err
	}

	for i := range next.Tables {
		nt := &next.Tables[i]

		t := s.Table(nt.Name)
		if t == nil {
			// Deep enough that the schemas never share a backing array: a later Merge appends columns to the added table.
			added := *nt
			added.ID = TableID(len(s.Tables))
			added.Columns = slices.Clone(nt.Columns)
			added.PK.Composite = slices.Clone(nt.PK.Composite)
			added.Children = nil
			s.Tables = append(s.Tables, added)
			changed = true
			continue
		}

		for k := len(t.Columns); k < len(nt.Columns); k++ {
			column := nt.Columns[k]
			column.ID = ColumnID(k)
			t.Columns = append(t.Columns, column)
			changed = true
		}
	}

	if changed {
		s.version++
		s.index()
		for i := range s.Tables {
			s.maxColumns = max(s.maxColumns, len(s.Tables[i].Columns))
		}
	}
	return changed, nil
}

// SchemaStore persists the accumulated schema of a discovering producer.
//
// Save it when [Discoverable.SchemaVersion] increased and before the cursor is persisted: a schema ahead of the cursor is
// harmless, the reverse could lose columns the rows already delivered rely on.
type SchemaStore interface {
	// Load returns the saved schema, or nil and no error when nothing was saved yet.
	Load(ctx context.Context) (*Schema, error)
	Save(ctx context.Context, s *Schema) error
}

// NewFileSchemaStore stores the schema in one file, written atomically.
func NewFileSchemaStore(path string) SchemaStore { return fileSchemaStore{path: path} }

type fileSchemaStore struct{ path string }

func (f fileSchemaStore) Load(context.Context) (*Schema, error) {
	data, err := os.ReadFile(f.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	return UnmarshalSchema(data)
}

func (f fileSchemaStore) Save(_ context.Context, s *Schema) error {
	data, err := s.Marshal()
	if err != nil {
		return err
	}
	return writeFileAtomic(f.path, data)
}

// writeFileAtomic writes to a temp file next to path, syncs it and renames it over path, so a reader never sees a partial file.
func writeFileAtomic(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".schema_*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}
