package pgcopy

import (
	"fmt"
	"math/big"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	sqlbytes "github.com/streamingfast/substreams/sink/sql/bytes"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Normalize converts a value produced by the protobuf walker into something the
// pgtype binary encoder can write for the given column OID.
//
// Binary COPY does no coercion, so the conversions that the text path gets for
// free have to happen here. In particular an uint64 must not be sent as an int8
// to a NUMERIC column: it would be rejected outright, and values above 2^63 would
// be wrong even if it were not.
func Normalize(oid uint32, value any) (any, error) {
	return normalize(oid, value, sqlbytes.EncodingRaw)
}

// NormalizeWithEncoding converts a value for binary COPY while applying the configured
// protobuf-bytes representation. Non-raw bytes columns are text columns, so their payload
// must be encoded before the COPY encoder sees it.
func NormalizeWithEncoding(oid uint32, value any, encoding sqlbytes.Encoding) (any, error) {
	return normalize(oid, value, encoding)
}

func normalize(oid uint32, value any, encoding sqlbytes.Encoding) (any, error) {
	switch v := value.(type) {
	case nil:
		return nil, nil

	case uint64:
		if oid == pgtype.NumericOID {
			return numericFromBigInt(new(big.Int).SetUint64(v)), nil
		}
		return v, nil

	case uint32:
		if oid == pgtype.NumericOID {
			return numericFromBigInt(new(big.Int).SetUint64(uint64(v))), nil
		}
		return v, nil

	case uint:
		if oid == pgtype.NumericOID {
			return numericFromBigInt(new(big.Int).SetUint64(uint64(v))), nil
		}
		return v, nil

	case *big.Int:
		return numericFromBigInt(v), nil

	case *timestamppb.Timestamp:
		if v == nil {
			return nil, nil
		}
		return v.AsTime().UTC(), nil

	case time.Time:
		return v.UTC(), nil

	case string:
		// A NUMERIC column fed from a proto string field (the int128/uint256/decimal
		// conversions) arrives here as text. Empty means "no value" upstream, which the
		// row inserter turns into 0; keep that behaviour rather than writing NULL.
		if oid == pgtype.NumericOID {
			var n pgtype.Numeric
			if v == "" {
				v = "0"
			}
			if err := n.Scan(v); err != nil {
				return nil, fmt.Errorf("parsing %q as numeric: %w", v, err)
			}
			return n, nil
		}
		return v, nil

	case []byte:
		encoded, err := encoding.EncodeBytes(v)
		if err != nil {
			return nil, fmt.Errorf("encoding bytes: %w", err)
		}
		return encoded, nil

	case []any:
		return normalizeSlice(oid, v, encoding)

	default:
		return value, nil
	}
}

// NormalizeRow applies [Normalize] to every value of a row, in place.
func NormalizeRow(cols []Column, values []any) error {
	return NormalizeRowWithEncoding(cols, values, sqlbytes.EncodingRaw)
}

// NormalizeRowWithEncoding applies NormalizeWithEncoding to every value of a row, in place.
func NormalizeRowWithEncoding(cols []Column, values []any, encoding sqlbytes.Encoding) error {
	if len(cols) != len(values) {
		return fmt.Errorf("expected %d values, got %d", len(cols), len(values))
	}

	for i := range values {
		normalized, err := NormalizeWithEncoding(cols[i].OID, values[i], encoding)
		if err != nil {
			return fmt.Errorf("column %q: %w", cols[i].Name, err)
		}
		values[i] = normalized
	}

	return nil
}

// normalizeSlice turns the walker's []any array into a concretely typed slice, which
// the pgtype array codec can then encode against the array's element OID.
func normalizeSlice(oid uint32, in []any, encoding sqlbytes.Encoding) (any, error) {
	if len(in) == 0 {
		// Element type does not matter for an empty array, but the slice must still be
		// typed for the codec to find a plan.
		return []string{}, nil
	}

	switch in[0].(type) {
	case string:
		if elementOID(oid) == pgtype.NumericOID {
			out := make([]any, len(in))
			for i, v := range in {
				s, ok := v.(string)
				if !ok {
					return nil, fmt.Errorf("mixed element types in array: %T and string", v)
				}
				normalized, err := normalize(pgtype.NumericOID, s, encoding)
				if err != nil {
					return nil, fmt.Errorf("normalizing array element %d: %w", i, err)
				}
				out[i] = normalized
			}
			return out, nil
		}

		out := make([]string, len(in))
		for i, v := range in {
			s, ok := v.(string)
			if !ok {
				return nil, fmt.Errorf("mixed element types in array: %T and string", v)
			}
			out[i] = s
		}
		return out, nil

	case []byte:
		if encoding.IsStringType() {
			out := make([]string, len(in))
			for i, v := range in {
				b, ok := v.([]byte)
				if !ok {
					return nil, fmt.Errorf("mixed element types in array: %T and []byte", v)
				}
				encoded, err := encoding.EncodeBytes(b)
				if err != nil {
					return nil, fmt.Errorf("encoding array element %d: %w", i, err)
				}
				out[i] = encoded.(string)
			}
			return out, nil
		}

		out := make([][]byte, len(in))
		for i, v := range in {
			b, ok := v.([]byte)
			if !ok {
				return nil, fmt.Errorf("mixed element types in array: %T and []byte", v)
			}
			out[i] = b
		}
		return out, nil

	case int32, int64, uint32, uint64, float32, float64, bool:
		out := make([]any, len(in))
		for i, v := range in {
			normalized, err := Normalize(elementOID(oid), v)
			if err != nil {
				return nil, err
			}
			out[i] = normalized
		}
		return out, nil

	default:
		return nil, fmt.Errorf("unsupported array element type %T", in[0])
	}
}

// elementOID maps the well-known array OIDs back to their element OID, so that
// numeric-typed elements get the same treatment as a scalar column would.
func elementOID(arrayOID uint32) uint32 {
	switch arrayOID {
	case pgtype.NumericArrayOID:
		return pgtype.NumericOID
	case pgtype.Int8ArrayOID:
		return pgtype.Int8OID
	case pgtype.Int4ArrayOID:
		return pgtype.Int4OID
	case pgtype.TextArrayOID:
		return pgtype.TextOID
	case pgtype.ByteaArrayOID:
		return pgtype.ByteaOID
	default:
		return arrayOID
	}
}

func numericFromBigInt(v *big.Int) pgtype.Numeric {
	return pgtype.Numeric{Int: v, Exp: 0, Valid: true}
}
