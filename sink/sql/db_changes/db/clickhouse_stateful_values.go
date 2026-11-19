package db

import (
	"encoding/json"
	"fmt"
	"math/big"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	shopspring "github.com/shopspring/decimal"
)

// bigIntType is the scan type of ClickHouse's big integer columns (Int128 to UInt256).
var bigIntType = reflect.TypeFor[*big.Int]()

// statefulColumnValue converts a row value for ClickHouse, NULL becoming the type's zero value in
// a column that is not Nullable. Values are converted as ClickHouse stores them, so a key matches
// its row whatever its spelling: times cut to the column's precision, FixedString padded, UUIDs
// parsed. Times are RFC 3339, "2006-01-02 15:04:05[.fraction]", "2006-01-02" or Unix seconds.
func statefulColumnValue(c *ColumnInfo, value *string) (any, error) {
	nullable := isNullableColumnType(c.databaseTypeName)
	t := c.scanType
	if t.Kind() == reflect.Pointer && t != bigIntType {
		t = t.Elem()
	}

	if value == nil {
		if nullable {
			return nil, nil
		}
		switch t {
		case bigIntType:
			return new(big.Int), nil
		case reflectTypeTime:
			return time.Unix(0, 0).UTC(), nil
		}
		return reflect.Zero(t).Interface(), nil
	}

	switch {
	case t == reflectTypeTime:
		parsed, err := parseClickhouseTime(*value)
		if err != nil {
			return nil, err
		}
		return parsed.Truncate(timePrecision(c.databaseTypeName)), nil
	case t.Kind() == reflect.String:
		if n := fixedStringLength(c.databaseTypeName); n > len(*value) {
			return *value + strings.Repeat("\x00", n-len(*value)), nil
		}
		return *value, nil
	case t == reflect.TypeFor[uuid.UUID]():
		return uuid.Parse(*value)
	case t == reflect.TypeFor[shopspring.Decimal]():
		return shopspring.NewFromString(*value)
	case t.Kind() == reflect.Float32:
		f, err := strconv.ParseFloat(*value, 32)
		return float32(f), err
	}
	return convertToType(*value, t)
}

var (
	fixedStringTypeRegex = regexp.MustCompile(`^FixedString\((\d+)\)$`)
	dateTime64TypeRegex  = regexp.MustCompile(`^DateTime64\((\d+)`)
)

// fixedStringLength is the length of a FixedString(N) column type, 0 for other types.
func fixedStringLength(columnType string) int {
	if m := fixedStringTypeRegex.FindStringSubmatch(unwrapColumnType(columnType)); m != nil {
		n, _ := strconv.Atoi(m[1])
		return n
	}
	return 0
}

// timePrecision is the precision a ClickHouse time column type stores: a day for Date, a second
// for DateTime, 10^-P second for DateTime64(P).
func timePrecision(columnType string) time.Duration {
	t := unwrapColumnType(columnType)
	if t == "Date" || t == "Date32" {
		return 24 * time.Hour
	}
	if m := dateTime64TypeRegex.FindStringSubmatch(t); m != nil {
		precision, _ := strconv.Atoi(m[1])
		d := time.Duration(1)
		for range max(0, 9-precision) {
			d *= 10
		}
		return d
	}
	return time.Second
}

// isNullableColumnType reports whether a column type is Nullable, under LowCardinality too.
func isNullableColumnType(t string) bool {
	t = strings.TrimSpace(t)
	if strings.HasPrefix(t, "LowCardinality(") && strings.HasSuffix(t, ")") {
		t = strings.TrimSpace(t[len("LowCardinality(") : len(t)-1])
	}
	return strings.HasPrefix(t, "Nullable(")
}

func parseClickhouseTime(value string) (time.Time, error) {
	if integerRegex.MatchString(value) {
		seconds, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return time.Time{}, err
		}
		return time.Unix(seconds, 0).UTC(), nil
	}
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02 15:04:05.999999999", "2006-01-02"} {
		if t, err := time.Parse(layout, value); err == nil {
			return t.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("cannot read %q as a time", value)
}

// formatClickhouseValue is the inverse of statefulColumnValue.
func formatClickhouseValue(v any, c *ColumnInfo) (*string, error) {
	rv := reflect.ValueOf(v)
	for rv.IsValid() && rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return nil, nil
		}
		if b, ok := reflect.TypeAssert[*big.Int](rv); ok {
			s := b.String()
			return &s, nil
		}
		rv = rv.Elem()
	}
	if !rv.IsValid() {
		return nil, nil
	}

	var s string
	switch value := rv.Interface().(type) {
	case string:
		s = value
	case bool:
		s = strconv.FormatBool(value)
	case big.Int:
		s = value.String()
	case shopspring.Decimal:
		s = value.String()
	case time.Time:
		if t := unwrapColumnType(c.databaseTypeName); t == "Date" || t == "Date32" {
			s = value.Format("2006-01-02")
		} else {
			s = value.UTC().Format(time.RFC3339Nano)
		}
	case float32:
		s = strconv.FormatFloat(float64(value), 'g', -1, 32)
	case float64:
		s = strconv.FormatFloat(value, 'g', -1, 64)
	case fmt.Stringer:
		s = value.String()
	default:
		switch rv.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			s = strconv.FormatInt(rv.Int(), 10)
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			s = strconv.FormatUint(rv.Uint(), 10)
		case reflect.Slice, reflect.Array:
			encoded, err := json.Marshal(value)
			if err != nil {
				return nil, fmt.Errorf("encoding %T: %w", value, err)
			}
			s = string(encoded)
		default:
			return nil, fmt.Errorf("unsupported value type %T", value)
		}
	}
	return &s, nil
}
