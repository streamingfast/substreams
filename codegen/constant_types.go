package codegen

type SubgraphType string

const (
	Bytes      SubgraphType = "Bytes"
	String     SubgraphType = "String"
	Boolean    SubgraphType = "Boolean"
	Int        SubgraphType = "Int"
	Int8       SubgraphType = "Int8"
	BigInt     SubgraphType = "BigInt"
	BigDecimal SubgraphType = "BigDecimal"
	Timestamp  SubgraphType = "Timestamp" // this is an alias for i64
)

func (t SubgraphType) String() string {
	return string(t)
}

type SqlType int

/*
VARCHAR(40)
BOOL
TEXT
INT
DECIMAL
INT
DECIMAL
DECIMAL
*/

type ClickhouseType int

/*
VARCHAR(40)
BOOL
TEXT
Int8
Int16
Int32
Int64
Int128
Int256
UInt8
UInt16
UInt32
UInt64
UInt128
UInt256

Decimal128(%d) -- the %d is the precision
Decimal128(%d)
Decimal128(%d)
Decimal256(%d)
Decimal32(%d)
Decimal64(%d)
Decimal128(%d)
Decimal256(%d)
Array(%s) -- the %s is the type
*/
