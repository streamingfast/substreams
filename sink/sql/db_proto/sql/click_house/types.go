package clickhouse

import (
	"fmt"

	"github.com/ClickHouse/ch-go/proto"
	v1 "github.com/streamingfast/substreams/pb/sf/substreams/sink/sql/schema/v1"
	"github.com/streamingfast/substreams/sink/sql/bytes"
	"github.com/streamingfast/substreams/sink/sql/db_proto/sql/schema"
	"google.golang.org/protobuf/reflect/protoreflect"
)

type DataType string

const (
	TypeInteger8   DataType = "Int8"
	TypeInteger16  DataType = "Int16"
	TypeInteger32  DataType = "Int32"
	TypeInteger64  DataType = "Int64"
	TypeInteger128 DataType = "Int128"
	TypeInteger256 DataType = "Int256"

	TypeUInt8   DataType = "UInt8"
	TypeUInt16  DataType = "UInt16"
	TypeUInt32  DataType = "UInt32"
	TypeUInt64  DataType = "UInt64"
	TypeUInt128 DataType = "UInt128"
	TypeUInt256 DataType = "UInt256"

	TypeFloat32 DataType = "Float32"
	TypeFloat64 DataType = "Float64"

	TypeDecimal128 = "Decimal128"
	TypeDecimal256 = "Decimal256"

	TypeBool    DataType = "Bool"
	TypeVarchar DataType = "VARCHAR"

	TypeDateTime DataType = "DateTime"
)

func (s DataType) String() string {
	return string(s)
}

func MapFieldType(fd protoreflect.FieldDescriptor, bytesEncoding bytes.Encoding, column *schema.Column) DataType {
	kind := fd.Kind()
	var baseType DataType

	switch kind {
	case protoreflect.MessageKind:
		switch {
		case fd.Message().FullName() == "google.protobuf.Timestamp":
			baseType = TypeDateTime
		case column.Nested != nil:
			// Nested columns are handled separately by dialect.go using ClickHouse Nested() syntax
			// This case should not be reached in normal flow as nested columns are processed differently
			return DataType("")
		default:
			panic(fmt.Sprintf("Message type not supported: %s", string(fd.Message().FullName())))
		}
	case protoreflect.EnumKind:
		baseType = TypeInteger32
	case protoreflect.BoolKind:
		baseType = TypeBool
	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind:
		baseType = TypeInteger32
	case protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind:
		baseType = TypeInteger64
	case protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		baseType = TypeUInt64
	case protoreflect.Uint32Kind, protoreflect.Fixed32Kind:
		baseType = TypeUInt32
	case protoreflect.FloatKind:
		baseType = TypeFloat32
	case protoreflect.DoubleKind:
		baseType = TypeFloat64
	case protoreflect.StringKind:
		if column.ConvertTo != nil && column.ConvertTo.Convertion != nil {
			switch column.ConvertTo.Convertion.(type) {
			case *v1.StringConvertion_Int128:
				baseType = TypeInteger128
			case *v1.StringConvertion_Uint128:
				baseType = TypeUInt128
			case *v1.StringConvertion_Int256:
				baseType = TypeInteger256
			case *v1.StringConvertion_Uint256:
				baseType = TypeUInt256
			case *v1.StringConvertion_Decimal128:
				decimal128Conv := column.ConvertTo.Convertion.(*v1.StringConvertion_Decimal128)
				baseType = DataType(fmt.Sprintf("Decimal128(%d)", decimal128Conv.Decimal128.Scale))
			case *v1.StringConvertion_Decimal256:
				decimal256Conv := column.ConvertTo.Convertion.(*v1.StringConvertion_Decimal256)
				baseType = DataType(fmt.Sprintf("Decimal256(%d)", decimal256Conv.Decimal256.Scale))
			default:
				panic(fmt.Sprintf("unsupported type: %s", kind))
			}
		} else {
			baseType = TypeVarchar
		}

	case protoreflect.BytesKind:
		baseType = TypeVarchar
	default:
		panic(fmt.Sprintf("unsupported type: %s", kind))
	}

	// If field is repeated, wrap the base type as an array
	if fd.IsList() {
		return DataType(fmt.Sprintf("Array(%s)", baseType))
	}

	//if fd.IsProto3Optional() {
	//	return DataType(fmt.Sprintf("Nullable(%s)", baseType))
	//}

	return baseType
}

func ColInputForColumn(fd protoreflect.FieldDescriptor, bytesEncoding bytes.Encoding, column *schema.Column) proto.ColInput {
	var baseInput proto.ColInput

	switch fd.Kind() {
	case protoreflect.MessageKind:
		switch {
		case fd.Message().FullName() == "google.protobuf.Timestamp":
			baseInput = &proto.ColDateTime{}
		case column.Nested != nil:
			// Nested columns are handled separately by dialect.go using ClickHouse Nested() syntax
			// Return nil as these columns don't need ColInput
			return nil
		default:
			panic(fmt.Sprintf("Message type not supported: %s", string(fd.Message().FullName())))
		}
	case protoreflect.EnumKind:
		baseInput = &proto.ColInt32{}
	case protoreflect.BoolKind:
		baseInput = &proto.ColBool{}
	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind:
		baseInput = &proto.ColInt32{}
	case protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind:
		baseInput = &proto.ColInt64{}
	case protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		baseInput = &proto.ColUInt64{}
	case protoreflect.Uint32Kind, protoreflect.Fixed32Kind:
		baseInput = &proto.ColUInt32{}
	case protoreflect.FloatKind:
		baseInput = &proto.ColFloat32{}
	case protoreflect.DoubleKind:
		baseInput = &proto.ColFloat64{}
	case protoreflect.StringKind:
		if column.ConvertTo != nil && column.ConvertTo.Convertion != nil {
			switch column.ConvertTo.Convertion.(type) {
			case *v1.StringConvertion_Int128:
				baseInput = &proto.ColInt128{}
			case *v1.StringConvertion_Uint128:
				baseInput = &proto.ColUInt128{}
			case *v1.StringConvertion_Int256:
				baseInput = &proto.ColInt256{}
			case *v1.StringConvertion_Uint256:
				baseInput = &proto.ColUInt256{}
			case *v1.StringConvertion_Decimal128:
				innerCol := &proto.ColDecimal128{}
				scale := (column.ConvertTo.Convertion.(*v1.StringConvertion_Decimal128)).Decimal128.Scale
				baseInput = &ColScaledDecimal128{
					ColDecimal128: innerCol,
					scale:         uint8(scale),
				}
			case *v1.StringConvertion_Decimal256:
				innerCol := &proto.ColDecimal256{}
				scale := (column.ConvertTo.Convertion.(*v1.StringConvertion_Decimal256)).Decimal256.Scale
				baseInput = &ColScaledDecimal256{
					ColDecimal256: innerCol,
					scale:         uint8(scale),
				}
			default:
				panic(fmt.Sprintf("unsupported type: %s", fd.Kind()))
			}
		} else {
			baseInput = &proto.ColStr{}
		}
	case protoreflect.BytesKind:
		if bytesEncoding.IsStringType() {
			baseInput = &proto.ColStr{}
		} else {
			baseInput = &proto.ColBytes{}
		}
	default:
		panic(fmt.Sprintf("unsupported type: %s", fd.Kind()))
	}

	// If field is repeated, wrap the base input as an array
	if fd.IsList() {
		switch base := baseInput.(type) {
		case *proto.ColInt32:
			return proto.NewArray(base)
		case *proto.ColInt64:
			return proto.NewArray(base)
		case *proto.ColUInt32:
			return proto.NewArray(base)
		case *proto.ColUInt64:
			return proto.NewArray(base)
		case *proto.ColFloat32:
			return proto.NewArray(base)
		case *proto.ColFloat64:
			return proto.NewArray(base)
		case *proto.ColBool:
			return proto.NewArray(base)
		case *proto.ColStr:
			return proto.NewArray(base)
		case *proto.ColBytes:
			return proto.NewArray(base)
		case *proto.ColDateTime:
			return proto.NewArray(base)
		default:
			panic(fmt.Sprintf("unsupported array base type: %T", base))
		}
	}

	return baseInput
}
