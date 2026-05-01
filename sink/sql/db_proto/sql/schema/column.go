package schema

import (
	"fmt"
	"strings"

	v1 "github.com/streamingfast/substreams/sink/sql/pb/sf/substreams/sink/sql/schema/v1"
	"google.golang.org/protobuf/reflect/protoreflect"
)

type Column struct {
	Name            string
	ForeignKey      *ForeignKey
	FieldDescriptor protoreflect.FieldDescriptor
	IsPrimaryKey    bool
	IsUnique        bool
	IsRepeated      bool
	IsExtension     bool
	IsMessage       bool
	IsOptional      bool
	Nested          *Table
	Message         string
	ConvertTo       *v1.StringConvertion
}

func NewColumn(d protoreflect.FieldDescriptor, fieldInfo *v1.Column, ordinal int, inlineDepth int) (*Column, error) {
	out := &Column{
		Name:            string(d.Name()),
		FieldDescriptor: d,
		IsRepeated:      d.IsList(),
		IsMessage:       d.Kind() == protoreflect.MessageKind,
		IsExtension:     d.IsExtension(),
		IsOptional:      d.HasOptionalKeyword(),
	}

	if fieldInfo != nil {
		if fieldInfo.Inline {
			if inlineDepth >= 1 {
				return nil, fmt.Errorf("inline nesting level %d is not supported for column %q: only 1 level of inline nesting is allowed", inlineDepth+1, out.Name)
			}
			ti := &v1.Table{
				Name: out.Name,
			}
			nested, err := NewTable(d.Message(), ti, ordinal+1, inlineDepth+1)
			if err != nil {
				return nil, fmt.Errorf("creating nested column %s: %w", out.Name, err)
			}
			out.Nested = nested
		}

		if fieldInfo.Name != nil {
			out.Name = *fieldInfo.Name
		}
		if fieldInfo.ForeignKey != nil {
			fk, err := NewForeignKey(*fieldInfo.ForeignKey)
			if err != nil {
				return nil, fmt.Errorf("error parsing foreign key %s: %w", *fieldInfo.ForeignKey, err)
			}
			out.ForeignKey = fk
		}
		out.IsPrimaryKey = fieldInfo.PrimaryKey
		out.IsUnique = fieldInfo.Unique
		out.ConvertTo = fieldInfo.ConvertTo
	}

	if out.IsMessage {
		out.Message = string(d.Message().Name())
	}
	return out, nil
}

func (c *Column) QuotedName() string {
	return fmt.Sprintf("%q", c.Name)
}

type ForeignKey struct {
	Table      string
	TableField string
}

func NewForeignKey(foreignKey string) (*ForeignKey, error) {
	parts := strings.Split(foreignKey, " on ")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid foreign key format %q. expecting 'table_name on field_name' format", foreignKey)
	}
	return &ForeignKey{
		Table:      strings.TrimSpace(parts[0]),
		TableField: strings.TrimSpace(parts[1]),
	}, nil
}
