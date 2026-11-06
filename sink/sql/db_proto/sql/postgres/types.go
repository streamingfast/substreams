package postgres

import (
	"encoding/hex"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	v1 "github.com/streamingfast/substreams/pb/sf/substreams/sink/sql/schema/v1"
	"github.com/streamingfast/substreams/sink/sql/bytes"
	sql2 "github.com/streamingfast/substreams/sink/sql/db_proto/sql"
	"github.com/streamingfast/substreams/sink/sql/db_proto/sql/schema"
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

// quoteLiteral renders a Go string as a PostgreSQL string literal.
//
// Only the quote is doubled. A backslash carries no meaning inside a standard string
// literal, and standard_conforming_strings has been on by default since PostgreSQL 9.1, so
// escaping it stored two backslashes where the value had one. That also made the write
// mode visible in the data: the COPY path hands pgtype the string as it is, and only the
// rendered paths came through here.
func quoteLiteral(v string) string {
	return "'" + strings.ReplaceAll(v, "'", "''") + "'"
}

// pgFloatText is a float's text input form. NaN and the infinities are words, which as
// SQL have to be quoted: bare, `NaN` parses as a column name.
func pgFloatText(v float64, bitSize int) (text string, isWord bool) {
	switch {
	case math.IsNaN(v):
		return "NaN", true
	case math.IsInf(v, 1):
		return "Infinity", true
	case math.IsInf(v, -1):
		return "-Infinity", true
	}

	return strconv.FormatFloat(v, 'f', -1, bitSize), false
}

func pgFloatLiteral(v float64, bitSize int) string {
	text, isWord := pgFloatText(v, bitSize)
	if isWord {
		return "'" + text + "'"
	}

	return text
}

// pgTimestampText is a time's text input form for a TIMESTAMP column, in UTC and cut to the
// column's microsecond precision the same way binary COPY cuts it.
func pgTimestampText(t time.Time) string {
	return t.UTC().Truncate(time.Microsecond).Format("2006-01-02 15:04:05.999999")
}

func ValueToString(value any, bytesEncoding bytes.Encoding) (s string) {
	switch v := value.(type) {
	case nil:
		s = "NULL"
	case string:
		s = quoteLiteral(v)
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
		s = pgFloatLiteral(v, 64)
	case float32:
		s = pgFloatLiteral(float64(v), 32)
	case []uint8:
		if bytesEncoding == bytes.EncodingRaw {
			// bytea hex input format. Casting an E'<hex>' string instead would store the
			// characters of the hex representation, twice the intended byte count.
			s = `'\x` + hex.EncodeToString(v) + `'::bytea`
		} else {
			encoded, err := bytesEncoding.EncodeBytes(v)
			if err != nil {
				panic(fmt.Sprintf("failed to encode bytes: %v", err))
			}
			s = "'" + encoded.(string) + "'"
		}
	case bool:
		s = strconv.FormatBool(v)
	case sql2.EnumValue:
		s = quoteLiteral(v.String())
	case time.Time:
		s = "'" + pgTimestampText(v) + "'"
	case *timestamppb.Timestamp:
		s = "'" + pgTimestampText(v.AsTime()) + "'"
	case postgresTextArray:
		if v == nil {
			s = "NULL"
			return
		}
		literal, _ := v.Value()
		s = quoteLiteral(literal.(string))
	case []interface{}:
		// An untyped literal takes the column's type. An array[...] constructor takes its
		// elements' instead, so array['1'] is text[] and an INSERT into numeric[] fails.
		s = quoteLiteral(pgArrayLiteral(v, bytesEncoding))
	case protoreflect.Message:
		jsonBytes, err := protojson.Marshal(v.Interface())
		if err != nil {
			panic(fmt.Sprintf("failed to marshal protobuf message to JSON: %v", err))
		}
		s = quoteLiteral(string(jsonBytes))
		return
	default:
		if msg, ok := v.(protoreflect.ProtoMessage); ok {
			jsonBytes, err := protojson.Marshal(msg)
			if err != nil {
				panic(fmt.Sprintf("failed to marshal protobuf message to JSON: %v", err))
			}
			s = quoteLiteral(string(jsonBytes))
			return
		}
		panic(fmt.Sprintf("unsupported type: %T", v))
	}
	return
}
