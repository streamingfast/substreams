// Code generated stub for compilation purposes only.
package pbschema

import (
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/runtime/protoimpl"
)

var E_Table *protoimpl.ExtensionInfo
var E_Field *protoimpl.ExtensionInfo

type Function int32

const (
	Function_unset          Function = 0
	Function_toMonth        Function = 1
	Function_toDate         Function = 2
	Function_toStartOfMonth Function = 3
	Function_toYear         Function = 4
	Function_toYYYYDD       Function = 5
	Function_toYYYYMM       Function = 6
)

func (f Function) String() string {
	switch f {
	case Function_toMonth:
		return "toMonth"
	case Function_toDate:
		return "toDate"
	case Function_toStartOfMonth:
		return "toStartOfMonth"
	case Function_toYear:
		return "toYear"
	case Function_toYYYYDD:
		return "toYYYYDD"
	case Function_toYYYYMM:
		return "toYYYYMM"
	default:
		return "unknown"
	}
}

type IndexType string

func (t IndexType) String() string { return string(t) }

type Table struct {
	Name                   string
	ChildOf                *string
	ClickhouseTableOptions *ClickhouseTableOptions
}

func (t *Table) Reset()                        {}
func (t *Table) String() string                { return t.Name }
func (t *Table) ProtoMessage()                 {}
func (t *Table) ProtoReflect() protoreflect.Message { return nil }

type Column struct {
	Name       *string
	ForeignKey *string
	PrimaryKey bool
	Unique     bool
	ConvertTo  *StringConvertion
	Inline     bool
}

func (c *Column) Reset()                        {}
func (c *Column) String() string                { return "" }
func (c *Column) ProtoMessage()                 {}
func (c *Column) ProtoReflect() protoreflect.Message { return nil }

type StringConvertion struct {
	Convertion isStringConvertion_Convertion
}

func (s *StringConvertion) Reset()                        {}
func (s *StringConvertion) String() string                { return "" }
func (s *StringConvertion) ProtoMessage()                 {}
func (s *StringConvertion) ProtoReflect() protoreflect.Message { return nil }

type isStringConvertion_Convertion interface {
	isStringConvertion_Convertion()
}

type StringConvertion_Int128 struct{}

func (*StringConvertion_Int128) isStringConvertion_Convertion() {}

type StringConvertion_Uint128 struct{}

func (*StringConvertion_Uint128) isStringConvertion_Convertion() {}

type StringConvertion_Int256 struct{}

func (*StringConvertion_Int256) isStringConvertion_Convertion() {}

type StringConvertion_Uint256 struct{}

func (*StringConvertion_Uint256) isStringConvertion_Convertion() {}

type StringConvertion_Decimal128 struct {
	Decimal128 *Decimal128Precision
}

func (*StringConvertion_Decimal128) isStringConvertion_Convertion() {}

type StringConvertion_Decimal256 struct {
	Decimal256 *Decimal256Precision
}

func (*StringConvertion_Decimal256) isStringConvertion_Convertion() {}

type Decimal128Precision struct {
	Scale int32
}

func (d *Decimal128Precision) Reset()                        {}
func (d *Decimal128Precision) String() string                { return "" }
func (d *Decimal128Precision) ProtoMessage()                 {}
func (d *Decimal128Precision) ProtoReflect() protoreflect.Message { return nil }

type Decimal256Precision struct {
	Scale int32
}

func (d *Decimal256Precision) Reset()                        {}
func (d *Decimal256Precision) String() string                { return "" }
func (d *Decimal256Precision) ProtoMessage()                 {}
func (d *Decimal256Precision) ProtoReflect() protoreflect.Message { return nil }

type ClickhouseTableOptions struct {
	OrderByFields   []*ClickhouseOrderByField
	PartitionFields []*ClickhousePartitionField
	IndexFields     []*ClickhouseIndexField
}

func (c *ClickhouseTableOptions) Reset()                        {}
func (c *ClickhouseTableOptions) String() string                { return "" }
func (c *ClickhouseTableOptions) ProtoMessage()                 {}
func (c *ClickhouseTableOptions) ProtoReflect() protoreflect.Message { return nil }

type ClickhouseOrderByField struct {
	Name       string
	Descending bool
	Function   Function
}

func (c *ClickhouseOrderByField) Reset()                        {}
func (c *ClickhouseOrderByField) String() string                { return c.Name }
func (c *ClickhouseOrderByField) ProtoMessage()                 {}
func (c *ClickhouseOrderByField) ProtoReflect() protoreflect.Message { return nil }

type ClickhousePartitionField struct {
	Name     string
	Function Function
}

func (c *ClickhousePartitionField) Reset()                        {}
func (c *ClickhousePartitionField) String() string                { return c.Name }
func (c *ClickhousePartitionField) ProtoMessage()                 {}
func (c *ClickhousePartitionField) ProtoReflect() protoreflect.Message { return nil }

type ClickhouseIndexField struct {
	Name        string
	FieldName   string
	Function    Function
	Type        IndexType
	Granularity uint64
}

func (c *ClickhouseIndexField) Reset()                        {}
func (c *ClickhouseIndexField) String() string                { return c.Name }
func (c *ClickhouseIndexField) ProtoMessage()                 {}
func (c *ClickhouseIndexField) ProtoReflect() protoreflect.Message { return nil }
