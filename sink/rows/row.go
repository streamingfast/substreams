package rows

import (
	"fmt"
	"slices"
	"strconv"
	"time"

	"google.golang.org/protobuf/reflect/protoreflect"
)

// BlockRef is the block context handed to [RowProducer.Rows]. Rows carry a pointer to it, so a consumer gets
// the block number, id and timestamp (for example `_block_number_` and `_block_timestamp_` columns) without
// threading it separately. The package itself only uses it in error messages.
type BlockRef struct {
	Number    uint64
	ID        string
	Timestamp time.Time
}

// Present is a bitset over the columns of a row, one bit per [ColumnID]. Up to 64 columns it is a single inline word.
type Present struct {
	lo uint64
	hi []uint64
}

// Has tells whether the column has a value in the row.
func (p *Present) Has(c ColumnID) bool {
	if c < 64 {
		return p.lo&(1<<uint(c)) != 0
	}

	word := int(c)/64 - 1
	return word < len(p.hi) && p.hi[word]&(1<<(uint(c)%64)) != 0
}

func (p *Present) set(c ColumnID) {
	if c < 64 {
		p.lo |= 1 << uint(c)
		return
	}

	word := int(c)/64 - 1
	for len(p.hi) <= word {
		p.hi = append(p.hi, 0)
	}
	p.hi[word] |= 1 << (uint(c) % 64)
}

func (p *Present) reset() {
	p.lo = 0
	clear(p.hi)
}

func (p Present) clone() Present {
	return Present{lo: p.lo, hi: slices.Clone(p.hi)}
}

// Row is one row of one table, a view into the decoded block message.
//
// The producer owns the Row and reuses it (one frame per nesting depth, so [Row.Parent] stays valid for the
// whole subtree), which is what makes the walk allocation free. A row, its Values and everything reachable
// from them are only valid until the next step of the iteration. Use [Row.Detach] to keep a row.
type Row struct {
	Table *Table
	Op    Op
	// Index is the 0-based emission index of the row within its table, for the current block. It is
	// what a SQL consumer turns into a row id.
	Index uint32
	// SeqBlock is the 0-based emission index of the row across all tables, for the current block. Together
	// with the block it identifies the position of the row in the stream, and it is what lets a consumer
	// resume in the middle of a block, see [Stream].
	SeqBlock uint32
	// Block is the block the row belongs to, the same pointer for every row of a [RowProducer.Rows] call.
	Block *BlockRef
	// Values holds one value per column of Table, in column order. It is the zero Value when the column
	// is not present.
	Values []protoreflect.Value
	// Present says which columns have a value. It is false for an unset optional scalar, an unset message
	// (Timestamp, inline) and an unset oneof member; everything else, including empty lists and proto3
	// implicit zero values, is present.
	Present Present
	// Parent is the enclosing emitted row, nil for a top-level row.
	Parent *Row
}

// Value returns the value of a column and whether it is present.
func (r *Row) Value(c ColumnID) (protoreflect.Value, bool) {
	if !r.Present.Has(c) {
		return protoreflect.Value{}, false
	}
	return r.Values[c], true
}

// Key returns a view of the primary key columns of the row.
func (r *Row) Key() Key { return Key{Row: r} }

// Detach returns a copy that does not depend on the producer's frames: Values, Present, Block and the parent
// chain are copied. Strings and byte slices are NOT copied, they still alias the decoded message, so keep its
// arena alive or clone them with [strings.Clone] and [bytes.Clone].
func (r *Row) Detach() *Row {
	out := *r
	out.Values = slices.Clone(r.Values)
	out.Present = r.Present.clone()
	if r.Block != nil {
		block := *r.Block
		out.Block = &block
	}
	if r.Parent != nil {
		out.Parent = r.Parent.Detach()
	}
	return &out
}

// Key is a view of the primary key columns of a row, it allocates nothing.
type Key struct{ Row *Row }

// Len is the number of key columns, zero for a table without primary key.
func (k Key) Len() int { return k.Row.Table.PK.Len() }

// Column returns the i-th key column.
func (k Key) Column(i int) *Column { return &k.Row.Table.Columns[k.Row.Table.PK.At(i)] }

// Value returns the value of the i-th key column.
func (k Key) Value(i int) protoreflect.Value { return k.Row.Values[k.Row.Table.PK.At(i)] }

// AppendValue appends the canonical text of the i-th key column: integers in decimal, strings as is, bytes
// through enc (hex when enc is nil), booleans as true/false, enums as their number.
func (k Key) AppendValue(dst []byte, i int, enc func(dst, b []byte) []byte) ([]byte, error) {
	return AppendScalar(dst, k.Column(i).Type, k.Value(i), enc)
}

const hexDigits = "0123456789abcdef"

func appendHex(dst, b []byte) []byte {
	for _, c := range b {
		dst = append(dst, hexDigits[c>>4], hexDigits[c&0xf])
	}
	return dst
}

// AppendScalar appends the canonical text of a scalar value. Timestamps, structs and lists are not scalars and are an error.
func AppendScalar(dst []byte, t LogicalType, v protoreflect.Value, enc func(dst, b []byte) []byte) ([]byte, error) {
	switch t.Kind {
	case KindString, KindBigint, KindDecimal:
		return append(dst, v.String()...), nil
	case KindBool:
		return strconv.AppendBool(dst, v.Bool()), nil
	case KindInt32, KindInt64:
		return strconv.AppendInt(dst, v.Int(), 10), nil
	case KindUint32, KindUint64:
		return strconv.AppendUint(dst, v.Uint(), 10), nil
	case KindFloat32:
		return strconv.AppendFloat(dst, v.Float(), 'g', -1, 32), nil
	case KindFloat64:
		return strconv.AppendFloat(dst, v.Float(), 'g', -1, 64), nil
	case KindEnum:
		return strconv.AppendInt(dst, int64(v.Enum()), 10), nil
	case KindBytes:
		if enc == nil {
			enc = appendHex
		}
		return enc(dst, v.Bytes()), nil
	default:
		return dst, fmt.Errorf("%s is not a scalar kind", t.Kind)
	}
}
