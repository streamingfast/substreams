package codegen

type OutputType string

const (
	Subgraph OutputType = "subgraph"
	Sql      OutputType = "sql"
)

func (t OutputType) String() string {
	return string(t)
}

type SubgraphType string

var (
	SubgraphBytes      SubgraphType = "Bytes"
	SubgraphString     SubgraphType = "String"
	SubgraphBoolean    SubgraphType = "Boolean"
	SubgraphInt        SubgraphType = "Int"
	SubgraphInt8       SubgraphType = "Int8"
	SubgraphBigInt     SubgraphType = "BigInt"
	SubgraphBigDecimal SubgraphType = "BigDecimal"
	SubgraphTimestamp  SubgraphType = "Timestamp" // this is an alias for i64
)

func (t SubgraphType) String() string {
	return string(t)
}

type SQLType string

var (
	PostgresSqlText    SQLType = "TEXT"
	PostgresSqlBoolean SQLType = "BOOL"
	PostgresSqlInt     SQLType = "Int"
	PostgresSqlDecimal SQLType = "DECIMAL"
	PostgresSqlBytes   SQLType = "BYTEA"
	ClickhouseString   SQLType = "String"
	ClickhouseBoolean  SQLType = "Bool"
	ClickhouseUInt32   SQLType = "UInt32"
	ClickhouseInt32    SQLType = "Int32"
	ClickhouseUInt64   SQLType = "UInt64"
	ClickhouseInt64    SQLType = "Int64"
	ClickhouseDecimal  SQLType = "Decimal64(%d)"
)

/*
VARCHAR(40)
BOOL
TEXT
INT
DECIMAL
*/

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
