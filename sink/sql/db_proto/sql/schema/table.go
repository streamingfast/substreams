package schema

import (
	"fmt"
	"strings"

	pbSchmema "github.com/streamingfast/substreams/pb/sf/substreams/sink/sql/schema/v1"
	"github.com/streamingfast/substreams/sink/sql/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

type PrimaryKey struct {
	Name            string
	FieldDescriptor protoreflect.FieldDescriptor
	Index           int
}

type ChildOf struct {
	ParentTable      string
	ParentTableField string
}

func NewChildOf(childOf string) (*ChildOf, error) {
	parts := strings.Split(childOf, " on ")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid child of format %q. expecting 'table_name on field_name' format", childOf)
	}

	return &ChildOf{
		ParentTable:      strings.TrimSpace(parts[0]),
		ParentTableField: strings.TrimSpace(parts[1]),
	}, nil
}

type Table struct {
	Name        string
	PrimaryKey  *PrimaryKey
	ChildOf     *ChildOf
	Columns     []*Column
	Ordinal     int
	InlineDepth int
	PbTableInfo *pbSchmema.Table
}

func NewTable(descriptor protoreflect.MessageDescriptor, tableInfo *pbSchmema.Table, ordinal int, inlineDepth int) (*Table, error) {
	table := &Table{
		Name:        string(descriptor.Name()),
		Ordinal:     ordinal,
		InlineDepth: inlineDepth,
		PbTableInfo: tableInfo,
	}
	table.Name = tableInfo.Name

	typeName := string(descriptor.Name())
	isTimestamp := typeName == ".google.protobuf.Timestamp" || typeName == "Timestamp"
	if isTimestamp {
		return nil, nil
	}

	if tableInfo.ChildOf != nil {
		co, err := NewChildOf(*tableInfo.ChildOf)
		if err != nil {
			return nil, fmt.Errorf("error parsing child of: %w", err)
		}
		table.ChildOf = co
	}

	err := table.processColumns(descriptor)
	if err != nil {
		return nil, fmt.Errorf("error processing fields for table %q: %w", string(descriptor.Name()), err)
	}

	if len(table.Columns) == 0 {
		return nil, nil
	}

	return table, nil
}

func (t *Table) processColumns(descriptor protoreflect.MessageDescriptor) error {
	fields := descriptor.Fields()
	for idx := 0; idx < fields.Len(); idx++ {
		fieldDescriptor := fields.Get(idx)
		fieldInfo := proto.FieldInfo(fieldDescriptor)

		if fieldDescriptor.ContainingOneof() != nil && !fieldDescriptor.HasOptionalKeyword() {
			continue
		}

		if fieldDescriptor.IsList() {
			if fieldDescriptor.Kind() == protoreflect.MessageKind {
				if fieldInfo != nil && fieldInfo.Inline {
					// Allow inline repeated message fields to be processed as nested columns
				} else {
					// This will be handled by table relations
					continue
				}
			}
			// Allow repeated scalar fields to be processed as array columns
		}

		if fieldDescriptor.Kind() == protoreflect.MessageKind {
			typeName := string(fieldDescriptor.Message().Name())
			isTimestamp := typeName == ".google.protobuf.Timestamp" || typeName == "Timestamp"

			isInline := fieldInfo != nil && fieldInfo.Inline
			if !isTimestamp && !isInline {
				continue
			}
		}
		column, err := NewColumn(fieldDescriptor, fieldInfo, t.Ordinal, t.InlineDepth)
		if err != nil {
			return fmt.Errorf("error processing column %q: %w", string(fieldDescriptor.Name()), err)
		}

		if column.IsPrimaryKey {
			if t.PrimaryKey != nil {
				return fmt.Errorf("multiple field mark has primary keys are not supported")
			}

			t.PrimaryKey = &PrimaryKey{
				Name:            column.Name,
				FieldDescriptor: fieldDescriptor,
				Index:           idx,
			}
		}
		t.Columns = append(t.Columns, column)
	}

	return nil
}

// ColumnByFieldName returns the column matching the given protobuf field name, or nil if not found.
func (t *Table) ColumnByFieldName(fieldName string) *Column {
	for _, col := range t.Columns {
		if col.FieldDescriptor != nil && string(col.FieldDescriptor.Name()) == fieldName {
			return col
		}
	}
	return nil
}
