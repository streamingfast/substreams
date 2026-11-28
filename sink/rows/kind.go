package rows

import (
	"fmt"
	"strings"

	"google.golang.org/protobuf/reflect/protoreflect"
)

//go:generate go-enum -f=$GOFILE --marshal --names

// Kind is the logical kind of a column value.
//
// ENUM(
//
//	string = 1
//	bool
//	int32
//	int64
//	uint32
//	uint64
//	float32
//	float64
//	bytes
//	timestamp
//	enum
//	bigint
//	decimal
//	struct
//	list
//
// )
type Kind uint8

// LogicalType is the type of a column. It is a plain comparable value, a [Column] embeds it so nothing
// is heap allocated per type.
type LogicalType struct {
	Kind Kind
	// Elem is the element kind of a [KindList]. A protobuf list never holds a list, so one level is enough.
	Elem Kind
	// Bits is 128 or 256 for [KindBigint] and [KindDecimal] (also as list element).
	Bits uint16
	// Signed applies to [KindBigint].
	Signed bool
	// Scale applies to [KindDecimal].
	Scale int32
	// Enum is set for [KindEnum], or a list of enums. It points into the descriptor graph, copying it allocates nothing.
	Enum protoreflect.EnumDescriptor
	// Struct is set for [KindStruct], or a list of structs.
	Struct protoreflect.MessageDescriptor
}

// ElemType returns the type of a list element, or t itself when it is not a list.
func (t LogicalType) ElemType() LogicalType {
	if t.Kind != KindList {
		return t
	}

	t.Kind, t.Elem = t.Elem, 0
	return t
}

// String renders the type, for example `bigint(256,unsigned)`, `decimal(128,4)` or `list<string>`.
func (t LogicalType) String() string {
	switch t.Kind {
	case KindBigint:
		sign := "unsigned"
		if t.Signed {
			sign = "signed"
		}
		return fmt.Sprintf("bigint(%d,%s)", t.Bits, sign)
	case KindDecimal:
		return fmt.Sprintf("decimal(%d,%d)", t.Bits, t.Scale)
	case KindList:
		var sb strings.Builder
		sb.WriteString("list<")
		sb.WriteString(t.ElemType().String())
		sb.WriteString(">")
		return sb.String()
	case KindEnum, KindStruct:
		return t.Kind.String()
	default:
		return t.Kind.String()
	}
}
