package postgres

import (
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/streamingfast/substreams/sink/sql/bytes"
	"github.com/streamingfast/substreams/sink/sql/db_proto/sql/schema"
	v1 "github.com/streamingfast/substreams/pb/sf/substreams/sink/sql/schema/v1"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type DataType string

const (
	TypeNumeric   DataType = "NUMERIC"
	TypeInteger   DataType = "INTEGER"
	TypeBool      DataType = "BOOLEAN"
	TypeBigInt    DataType = "BIGINT"
	TypeDecimal   DataType = "DECIMAL"
	TypeDouble    DataType = "DOUBLE PRECISION"
	TypeText      DataType = "TEXT"
	TypeBlob      DataType = "BLOB"
	TypeVarchar   DataType = "VARCHAR(255)"
	TypeBytea     DataType = "BYTEA"
	TypeTimestamp DataType = "TIMESTAMP"
	TypeJSONB     DataType = "JSONB"
)

func (s DataType) String() string {
	return string(s)
}

func IsWellKnownType(fd protoreflect.FieldDescriptor) bool {
	if fd.Kind() != protoreflect.MessageKind {
		return false
	}
	switch string(fd.Message().FullName()) {
	case "google.protobuf.Timestamp":
		return true
	default:
		return false
	}
}

func MapFieldType(fd protoreflect.FieldDescriptor, bytesEncoding bytes.Encoding, column *schema.Column) DataType {
	kind := fd.Kind()
	var baseType DataType

	switch kind {
	case protoreflect.MessageKind:
		if column.Nested != nil {
			baseType = TypeJSONB
		} else {
			switch string(fd.Message().FullName()) {
			case "google.protobuf.Timestamp":
				baseType = TypeTimestamp
			default:
				panic(fmt.Sprintf("Message type not supported: %s", string(fd.Message().FullName())))
			}
		}
	case protoreflect.BoolKind:
		baseType = TypeBool
	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind:
		baseType = TypeInteger
	case protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind:
		baseType = TypeBigInt
	case protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		baseType = TypeNumeric
	case protoreflect.Uint32Kind, protoreflect.Fixed32Kind:
		baseType = TypeNumeric
	case protoreflect.FloatKind:
		baseType = TypeDecimal
	case protoreflect.DoubleKind:
		baseType = TypeDouble
	case protoreflect.StringKind:
		if column.ConvertTo != nil && column.ConvertTo.Convertion != nil {
			switch column.ConvertTo.Convertion.(type) {
			case *v1.StringConvertion_Int128:
				baseType = TypeNumeric
			case *v1.StringConvertion_Uint128:
				baseType = TypeNumeric
			case *v1.StringConvertion_Int256:
				baseType = TypeNumeric
			case *v1.StringConvertion_Uint256:
				baseType = TypeNumeric
			case *v1.StringConvertion_Decimal128:
				decimal128Conv := column.ConvertTo.Convertion.(*v1.StringConvertion_Decimal128)
				baseType = DataType(fmt.Sprintf("DECIMAL(38,%d)", decimal128Conv.Decimal128.Scale))
			case *v1.StringConvertion_Decimal256:
				decimal256Conv := column.ConvertTo.Convertion.(*v1.StringConvertion_Decimal256)
				baseType = DataType(fmt.Sprintf("DECIMAL(76,%d)", decimal256Conv.Decimal256.Scale))
			default:
				baseType = TypeVarchar
			}
		} else {
			baseType = TypeVarchar
		}
	case protoreflect.BytesKind:
		if bytesEncoding.IsStringType() {
			baseType = TypeText
		} else {
			baseType = TypeBytea
		}
	case protoreflect.EnumKind:
		baseType = TypeText
	default:
		panic(fmt.Sprintf("unsupported type: %s", kind))
	}

	// If field is repeated, wrap the base type as an array
	if fd.IsList() {
		return DataType(fmt.Sprintf("%s[]", baseType))
	}

	return baseType
}

func ValueToString(value any, bytesEncoding bytes.Encoding) (s string) {
	switch v := value.(type) {
	case string:
		s = "'" + strings.ReplaceAll(strings.ReplaceAll(v, "'", "''"), "\\", "\\\\") + "'"
	case int64:
		s = strconv.FormatInt(v, 10)
	case int32:
		s = strconv.FormatInt(int64(v), 10)
	case int:
		s = strconv.FormatInt(int64(v), 10)
	case uint64:
		s = strconv.FormatUint(v, 10)
	case uint32:
		s = strconv.FormatUint(uint64(v), 10)
	case uint:
		s = strconv.FormatUint(uint64(v), 10)
	case float64:
		s = strconv.FormatFloat(v, 'f', -1, 64)
	case float32:
		s = strconv.FormatFloat(float64(v), 'f', -1, 32)
	case []uint8:
		if bytesEncoding == bytes.EncodingRaw {
			s = "E'" + hex.EncodeToString(v) + "'::BYTEA"
		} else {
			encoded, err := bytesEncoding.EncodeBytes(v)
			if err != nil {
				panic(fmt.Sprintf("failed to encode bytes: %v", err))
			}
			s = "'" + encoded.(string) + "'"
		}
	case bool:
		s = strconv.FormatBool(v)
	case time.Time:
		s = "'" + v.Format(time.RFC3339) + "'"
	case *timestamppb.Timestamp:
		s = "'" + v.AsTime().Format(time.RFC3339) + "'"
	case []interface{}:
		if len(v) == 0 {
			s = "'{}'"
			return
		}

		var elements []string
		for _, elem := range v {
			elements = append(elements, ValueToString(elem, bytesEncoding))
		}
		s = "array[" + strings.Join(elements, ",") + "]"
	case protoreflect.Message:
		jsonBytes, err := protojson.Marshal(v.Interface())
		if err != nil {
			panic(fmt.Sprintf("failed to marshal protobuf message to JSON: %v", err))
		}
		s = "'" + strings.ReplaceAll(strings.ReplaceAll(string(jsonBytes), "'", "''"), "\\", "\\\\") + "'"
		return
	default:
		if msg, ok := v.(protoreflect.ProtoMessage); ok {
			jsonBytes, err := protojson.Marshal(msg)
			if err != nil {
				panic(fmt.Sprintf("failed to marshal protobuf message to JSON: %v", err))
			}
			s = "'" + strings.ReplaceAll(strings.ReplaceAll(string(jsonBytes), "'", "''"), "\\", "\\\\") + "'"
			return
		}
		panic(fmt.Sprintf("unsupported type: %T", v))
	}
	return
}
