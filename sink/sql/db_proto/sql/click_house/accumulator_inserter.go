package clickhouse

import (
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/ClickHouse/ch-go"
	"github.com/ClickHouse/ch-go/proto"
	"github.com/streamingfast/logging"
	"github.com/streamingfast/logging/zapx"
	v1 "github.com/streamingfast/substreams/pb/sf/substreams/sink/sql/schema/v1"
	"github.com/streamingfast/substreams/sink/sql/bytes"
	sql2 "github.com/streamingfast/substreams/sink/sql/db_proto/sql"
	"github.com/streamingfast/substreams/sink/sql/db_proto/sql/schema"
	"go.uber.org/zap"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type accumulator struct {
	ordinal   int
	tableName string
	columns   map[int]*schema.Column
	input     map[string]proto.ColInput
}

type AccumulatorInserter struct {
	accumulators  map[string]*accumulator
	cursorStmt    *sql.Stmt
	logger        *zap.Logger
	tracer        logging.Tracer
	bytesEncoding bytes.Encoding
}

func NewAccumulatorInserter(database *Database, logger *zap.Logger, tracer logging.Tracer) (*AccumulatorInserter, error) {
	logger = logger.Named("clickhouse inserter")

	accumulators, err := createAccumulators(database.dialect)
	if err != nil {
		return nil, fmt.Errorf("creating accumulators: %w", err)
	}
	return &AccumulatorInserter{
		accumulators:  accumulators,
		logger:        logger,
		tracer:        tracer,
		bytesEncoding: database.bytesEncoding,
	}, nil
}

func createAccumulators(dialect *DialectClickHouse) (map[string]*accumulator, error) {
	if dialect == nil {
		panic("dialect is nil")
	}

	accumulators := map[string]*accumulator{}

	accumulators[sql2.DialectTableBlock] = &accumulator{
		ordinal:   -1,
		tableName: sql2.DialectTableBlock,
		columns: map[int]*schema.Column{
			0: {Name: "number"},
			1: {Name: "hash"},
			2: {Name: "timestamp"},
			3: {Name: "version"},
			4: {Name: "deleted"},
		},
		input: map[string]proto.ColInput{
			"number":    &proto.ColUInt64{},
			"hash":      &proto.ColStr{},
			"timestamp": &proto.ColDateTime{},
			"version":   &proto.ColInt64{},
			"deleted":   &proto.ColBool{},
		},
	}

	tables := dialect.GetTables()
	for _, table := range tables {
		input := map[string]proto.ColInput{}
		columns := map[int]*schema.Column{}

		input[sql2.DialectFieldBlockNumber] = &proto.ColUInt64{}
		columns[0] = &schema.Column{Name: sql2.DialectFieldBlockNumber}

		input[sql2.DialectFieldBlockTimestamp] = &proto.ColDateTime{}
		columns[1] = &schema.Column{Name: sql2.DialectFieldBlockTimestamp}

		input[sql2.DialectFieldVersion] = &proto.ColInt64{}
		columns[2] = &schema.Column{Name: sql2.DialectFieldVersion}

		input[sql2.DialectFieldDeleted] = &proto.ColBool{}
		columns[3] = &schema.Column{Name: sql2.DialectFieldDeleted}

		if dialect.UseRowIDField(table.Name) {
			input[sql2.DialectFieldRowID] = &proto.ColUInt32{}
			columns[len(columns)] = &schema.Column{Name: sql2.DialectFieldRowID}
		}

		primaryName := ""
		if table.PrimaryKey != nil {
			pk := table.PrimaryKey
			primaryName = pk.Name

			input[pk.Name] = ColInputForColumn(pk.FieldDescriptor, dialect.bytesEncoding, table.Columns[pk.Index])
			columns[len(columns)] = &schema.Column{Name: pk.Name}
		}

		offset := len(columns)
		if table.ChildOf != nil {
			parentTable, parentFound := dialect.TableRegistry[table.ChildOf.ParentTable]
			if !parentFound {
				return nil, fmt.Errorf("parent table %q not found", table.ChildOf.ParentTable)
			}
			fieldFound := false
			for _, parentField := range parentTable.Columns {

				if parentField.Name == table.ChildOf.ParentTableField {
					input[parentField.Name] = ColInputForColumn(parentField.FieldDescriptor, dialect.bytesEncoding, parentField)
					columns[offset] = parentField
					fieldFound = true
					break
				}
			}
			if !fieldFound {
				return nil, fmt.Errorf("field %q not found in table %q", table.ChildOf.ParentTableField, table.ChildOf.ParentTable)
			}
		}

		offset = len(columns)
		skipCount := 0
		for i, column := range table.Columns {
			if column.Name == primaryName {
				skipCount++
				continue
			}
			// Handle nested columns with flatten_nested = 1 using dot notation
			if column.Nested != nil {
				// For each field in the nested table, create a proto.NewArray column with dot notation
				for _, nestedCol := range column.Nested.Columns {
					nestedColName := fmt.Sprintf("%s.%s", column.Name, nestedCol.Name)
					nestedInput := ColInputForColumn(nestedCol.FieldDescriptor, dialect.bytesEncoding, nestedCol)
					if nestedInput != nil {
						// Wrap in proto.NewArray for nested columns
						switch base := nestedInput.(type) {
						case *proto.ColStr:
							input[nestedColName] = proto.NewArray(base)
						case *proto.ColInt32:
							input[nestedColName] = proto.NewArray(base)
						case *proto.ColInt64:
							input[nestedColName] = proto.NewArray(base)
						case *proto.ColUInt32:
							input[nestedColName] = proto.NewArray(base)
						case *proto.ColUInt64:
							input[nestedColName] = proto.NewArray(base)
						case *proto.ColFloat32:
							input[nestedColName] = proto.NewArray(base)
						case *proto.ColFloat64:
							input[nestedColName] = proto.NewArray(base)
						case *proto.ColBool:
							input[nestedColName] = proto.NewArray(base)
						case *proto.ColBytes:
							input[nestedColName] = proto.NewArray(base)
						case *proto.ColDateTime:
							input[nestedColName] = proto.NewArray(base)
						default:
							return nil, fmt.Errorf("unsupported nested column type %T for column %s.%s", base, column.Name, nestedCol.Name)
						}
						// Create a pseudo-column entry for tracking
						nestedColEntry := &schema.Column{
							Name:            nestedColName,
							FieldDescriptor: nestedCol.FieldDescriptor,
						}
						columns[i+offset-skipCount] = nestedColEntry
						offset++
					}
				}
				skipCount++
				continue
			}
			input[column.Name] = ColInputForColumn(column.FieldDescriptor, dialect.bytesEncoding, column)
			columns[i+offset-skipCount] = column
		}

		accumulators[table.Name] = &accumulator{
			tableName: table.Name,
			ordinal:   table.Ordinal,
			columns:   columns,
			input:     input,
		}
	}

	return accumulators, nil
}

func (i *AccumulatorInserter) insert(table string, values []any) error {
	accumulator := i.accumulators[table]
	if accumulator == nil {
		return fmt.Errorf("accumulator not found for table %q", table)
	}
	if ce := i.logger.Check(zap.DebugLevel, "inserting"); ce != nil {
		ce.Write(zap.String("table", table), zap.Int("values", len(values)))
	}

	for idx, value := range values {
		column, found := accumulator.columns[idx]
		if !found {
			return fmt.Errorf("column not found for table %q at idx %d", table, idx)
		}
		input := accumulator.input[column.Name]

		if i.tracer.Enabled() {
			i.logger.Debug("inserting column value",
				zap.String("table", table),
				zap.String("column", column.Name),
				zapx.Type("column_type", input),
				zapx.Type("value_type", value),
			)
		}

		switch input := input.(type) {
		case *proto.ColDateTime:
			if t, ok := value.(*timestamppb.Timestamp); ok {
				input.Append(t.AsTime())
			} else if t, ok := value.(time.Time); ok {
				input.Append(t)
			} else {
				panic(fmt.Sprintf("unknown time base input type %T for column %s of table %s", input, column.Name, table))
			}
		case *proto.ColInt32:
			input.Append(int32Value(value))
		case *proto.ColInt64:
			input.Append(value.(int64))
		case *proto.ColUInt32:
			input.Append(value.(uint32))
		case *proto.ColUInt64:
			input.Append(value.(uint64))
		case *proto.ColFloat32:
			input.Append(value.(float32))
		case *proto.ColFloat64:
			input.Append(value.(float64))
		case *ColScaledDecimal128:
			stringValue := value.(string)
			scale := column.ConvertTo.Convertion.(*v1.StringConvertion_Decimal128).Decimal128.Scale
			// Handle optional fields with empty strings by using zero value
			if column.IsOptional && stringValue == "" {
				input.Append(proto.Decimal128{})
			} else {
				v, err := StringToDecimal128(stringValue, scale)
				if err != nil {
					panic(fmt.Sprintf("failed to convert string to decimal128 for column %s of table %s: %v", column.Name, table, err))
				}
				input.Append(v)
			}
		case *ColScaledDecimal256:
			stringValue := value.(string)
			scale := column.ConvertTo.Convertion.(*v1.StringConvertion_Decimal256).Decimal256.Scale
			// Handle optional fields with empty strings by using zero value
			if column.IsOptional && stringValue == "" {
				input.Append(proto.Decimal256{})
			} else {
				v, err := StringToDecimal256(stringValue, scale)
				if err != nil {
					panic(fmt.Sprintf("failed to convert string to decimal256 for column %s of table %s: %v", column.Name, table, err))
				}
				input.Append(v)
			}
		case *proto.ColInt128:
			stringValue := value.(string)
			// Handle optional fields with empty strings by using zero value
			if column.IsOptional && stringValue == "" {
				input.Append(proto.Int128{})
			} else {
				v, err := StringToInt128(stringValue)
				if err != nil {
					panic(fmt.Sprintf("failed to convert string to int128 for column %s of table %s: %v", column.Name, table, err))
				}
				input.Append(v)
			}
		case *proto.ColUInt128:
			stringValue := value.(string)
			// Handle optional fields with empty strings by using zero value
			if column.IsOptional && stringValue == "" {
				input.Append(proto.UInt128{})
			} else {
				v, err := StringToUInt128(stringValue)
				if err != nil {
					panic(fmt.Sprintf("failed to convert string to uint128 for column %s of table %s: %v", column.Name, table, err))
				}
				input.Append(v)
			}
		case *proto.ColInt256:
			stringValue := value.(string)
			// Handle optional fields with empty strings by using zero value
			if column.IsOptional && stringValue == "" {
				input.Append(proto.Int256{})
			} else {
				v, err := StringToInt256(stringValue)
				if err != nil {
					panic(fmt.Sprintf("failed to convert string to int256 for column %s of table %s: %v", column.Name, table, err))
				}
				input.Append(v)
			}
		case *proto.ColUInt256:
			stringValue := value.(string)
			// Handle optional fields with empty strings by using zero value
			if column.IsOptional && stringValue == "" {
				input.Append(proto.UInt256{})
			} else {
				v, err := StringToUInt256(stringValue)
				if err != nil {
					panic(fmt.Sprintf("failed to convert string to uint256 for column %s of table %s: %v", column.Name, table, err))
				}
				input.Append(v)
			}
		case *proto.ColStr:
			if bytesValue, ok := value.([]byte); ok {
				// Convert []byte to string using the bytes encoder
				encoded, err := i.bytesEncoding.EncodeBytes(bytesValue)
				if err != nil {
					panic(fmt.Sprintf("failed to encode bytes for column %s of table %s: %v", column.Name, table, err))
				}
				input.Append(encoded.(string))
			} else {
				input.Append(value.(string))
			}
		case *proto.ColBytes:
			input.Append(value.([]byte))
		case *proto.ColBool:
			input.Append(value.(bool))
		// Handle array column types
		case *proto.ColArr[int32]:
			if arr, ok := value.([]interface{}); ok {
				int32Arr := make([]int32, len(arr))
				for i, v := range arr {
					int32Arr[i] = int32Value(v)
				}
				input.Append(int32Arr)
			} else {
				panic(fmt.Sprintf("expected []interface{} for array column %s of table %s, got %T", column.Name, table, value))
			}
		case *proto.ColArr[int64]:
			if arr, ok := value.([]interface{}); ok {
				int64Arr := make([]int64, len(arr))
				for i, v := range arr {
					int64Arr[i] = v.(int64)
				}
				input.Append(int64Arr)
			} else {
				panic(fmt.Sprintf("expected []interface{} for array column %s of table %s, got %T", column.Name, table, value))
			}
		case *proto.ColArr[uint32]:
			if arr, ok := value.([]interface{}); ok {
				uint32Arr := make([]uint32, len(arr))
				for i, v := range arr {
					uint32Arr[i] = v.(uint32)
				}
				input.Append(uint32Arr)
			} else {
				panic(fmt.Sprintf("expected []interface{} for array column %s of table %s, got %T", column.Name, table, value))
			}
		case *proto.ColArr[uint64]:
			if arr, ok := value.([]interface{}); ok {
				uint64Arr := make([]uint64, len(arr))
				for i, v := range arr {
					uint64Arr[i] = v.(uint64)
				}
				input.Append(uint64Arr)
			} else {
				panic(fmt.Sprintf("expected []interface{} for array column %s of table %s, got %T", column.Name, table, value))
			}
		case *proto.ColArr[*proto.Int128]:
			if arr, ok := value.([]interface{}); ok {
				int128Arr := make([]*proto.Int128, len(arr))
				for i, v := range arr {
					stringValue := v.(string)
					// Handle empty strings in array elements by using zero value
					if stringValue == "" {
						zeroValue := proto.Int128{}
						int128Arr[i] = &zeroValue
					} else {
						v, err := StringToInt128(stringValue)
						if err != nil {
							panic(fmt.Sprintf("failed to convert array of string to int128 for column %s of table %s: %v", column.Name, table, err))
						}
						int128Arr[i] = &v
					}
				}
				input.Append(int128Arr)
			} else {
				panic(fmt.Sprintf("expected []interface{} for array column %s of table %s, got %T", column.Name, table, value))
			}
		case *proto.ColArr[*proto.UInt128]:
			if arr, ok := value.([]interface{}); ok {
				uint128Arr := make([]*proto.UInt128, len(arr))
				for i, v := range arr {
					stringValue := v.(string)
					// Handle empty strings in array elements by using zero value
					if stringValue == "" {
						zeroValue := proto.UInt128{}
						uint128Arr[i] = &zeroValue
					} else {
						v, err := StringToUInt128(stringValue)
						if err != nil {
							panic(fmt.Sprintf("failed to convert array of string to uint128 for column %s of table %s: %v", column.Name, table, err))
						}
						uint128Arr[i] = &v
					}
				}
				input.Append(uint128Arr)
			}
		case *proto.ColArr[*proto.Int256]:
			if arr, ok := value.([]interface{}); ok {
				int256Arr := make([]*proto.Int256, len(arr))
				for i, v := range arr {
					stringValue := v.(string)
					// Handle empty strings in array elements by using zero value
					if stringValue == "" {
						zeroValue := proto.Int256{}
						int256Arr[i] = &zeroValue
					} else {
						v, err := StringToInt256(stringValue)
						if err != nil {
							panic(fmt.Sprintf("failed to convert array of string to int256 for column %s of table %s: %v", column.Name, table, err))
						}
						int256Arr[i] = &v
					}
				}
				input.Append(int256Arr)
			}
		case *proto.ColArr[*proto.UInt256]:
			if arr, ok := value.([]interface{}); ok {
				uint256Arr := make([]*proto.UInt256, len(arr))
				for i, v := range arr {
					stringValue := v.(string)
					// Handle empty strings in array elements by using zero value
					if stringValue == "" {
						zeroValue := proto.UInt256{}
						uint256Arr[i] = &zeroValue
					} else {
						v, err := StringToUInt256(stringValue)
						if err != nil {
							panic(fmt.Sprintf("failed to convert array of string to uint256 for column %s of table %s: %v", column.Name, table, err))
						}
						uint256Arr[i] = &v
					}
				}
				input.Append(uint256Arr)
			}
		case *proto.ColArr[float32]:
			if arr, ok := value.([]interface{}); ok {
				float32Arr := make([]float32, len(arr))
				for i, v := range arr {
					float32Arr[i] = v.(float32)
				}
				input.Append(float32Arr)
			} else {
				panic(fmt.Sprintf("expected []interface{} for array column %s of table %s, got %T", column.Name, table, value))
			}
		case *proto.ColArr[float64]:
			if arr, ok := value.([]interface{}); ok {
				float64Arr := make([]float64, len(arr))
				for i, v := range arr {
					float64Arr[i] = v.(float64)
				}
				input.Append(float64Arr)
			} else {
				panic(fmt.Sprintf("expected []interface{} for array column %s of table %s, got %T", column.Name, table, value))
			}
		case *proto.ColArr[*proto.Decimal128]:
			scale := column.ConvertTo.Convertion.(*v1.StringConvertion_Decimal128).Decimal128.Scale
			if arr, ok := value.([]interface{}); ok {
				decimal128Arr := make([]*proto.Decimal128, len(arr))
				for i, v := range arr {
					v, err := StringToDecimal128(v.(string), scale)
					if err != nil {
						panic(fmt.Sprintf("failed to convert array of string to decimal128 for column %s of table %s: %v", column.Name, table, err))
					}
					decimal128Arr[i] = &v
				}
				input.Append(decimal128Arr)
			}
		case *proto.ColArr[*proto.Decimal256]:
			scale := column.ConvertTo.Convertion.(*v1.StringConvertion_Decimal128).Decimal128.Scale
			if arr, ok := value.([]interface{}); ok {
				decimal256Arr := make([]*proto.Decimal256, len(arr))
				for i, v := range arr {
					v, err := StringToDecimal256(v.(string), scale)
					if err != nil {
						panic(fmt.Sprintf("failed to convert array of string to decimal256 for column %s of table %s: %v", column.Name, table, err))
					}
					decimal256Arr[i] = &v
				}
				input.Append(decimal256Arr)
			}
		case *proto.ColArr[bool]:
			if arr, ok := value.([]interface{}); ok {
				boolArr := make([]bool, len(arr))
				for i, v := range arr {
					boolArr[i] = v.(bool)
				}
				input.Append(boolArr)
			} else {
				panic(fmt.Sprintf("expected []interface{} for array column %s of table %s, got %T", column.Name, table, value))
			}
		case *proto.ColArr[string]:
			if arr, ok := value.([]interface{}); ok {
				stringArr := make([]string, len(arr))
				for i, v := range arr {
					stringArr[i] = v.(string)
				}
				input.Append(stringArr)
			} else {
				panic(fmt.Sprintf("expected []interface{} for array column %s of table %s, got %T", column.Name, table, value))
			}
		case *proto.ColArr[[]byte]:
			if arr, ok := value.([]interface{}); ok {
				bytesArr := make([][]byte, len(arr))
				for i, v := range arr {
					bytesArr[i] = v.([]byte)
				}
				input.Append(bytesArr)
			} else {
				panic(fmt.Sprintf("expected []interface{} for array column %s of table %s, got %T", column.Name, table, value))
			}
		case *proto.ColArr[time.Time]:
			if arr, ok := value.([]interface{}); ok {
				timeArr := make([]time.Time, len(arr))
				for i, v := range arr {
					if t, ok := v.(*timestamppb.Timestamp); ok {
						timeArr[i] = t.AsTime()
					} else if t, ok := v.(time.Time); ok {
						timeArr[i] = t
					} else {
						panic(fmt.Sprintf("unknown time type %T in array for column %s of table %s", v, column.Name, table))
					}
				}
				input.Append(timeArr)
			} else {
				panic(fmt.Sprintf("expected []interface{} for array column %s of table %s, got %T", column.Name, table, value))
			}
		default:
			panic(fmt.Sprintf("unknown input type %T for column %s of table %s", input, column.Name, table))
		}
	}

	return nil
}

// int32Value narrows a value bound for an Int32 column, scalar or array element.
//
// An enum column is declared Int32 by MapFieldType, so the walk's EnumValue has to be
// reduced to its number here; a plain protoreflect.EnumNumber can still reach us from a
// path that did not go through sql.ScalarFieldValue.
func int32Value(value any) int32 {
	switch v := value.(type) {
	case sql2.EnumValue:
		return v.Number
	case protoreflect.EnumNumber:
		return int32(v)
	default:
		return value.(int32)
	}
}

func (i *AccumulatorInserter) flush(database *Database) error {
	i.logger.Debug("flushing started", zap.Int("accumulators", len(i.accumulators)))
	var accumulators []accumulator

	start := time.Now()
	for _, acc := range i.accumulators {
		accumulators = append(accumulators, *acc)
	}

	sort.Slice(accumulators, func(i, j int) bool {
		return accumulators[i].ordinal < accumulators[j].ordinal
	})

	client, err := database.client()
	if err != nil {
		return fmt.Errorf("clickhouse accumulator inserter: creating client: %w", err)
	}

	queryDuration := time.Duration(0)
	rowCount := 0
	for _, acc := range accumulators {
		qStart := time.Now()

		inputs := proto.Input{}
		for n, i := range acc.input {
			if n == "block_number" {
				rowCount += i.Rows()
			}
			inputs = append(inputs, proto.InputColumn{
				Name: n,
				Data: i,
			})
		}

		// Retry logic on client.Do failure: sleep between attempts and get a fresh client for each retry
		retryCount := database.queryRetryCount
		retrySleep := database.queryRetrySleep
		for attempt := 0; ; attempt++ {
			if err := client.Do(database.ctx, ch.Query{
				Body:  inputs.Into(acc.tableName), // helper that generates INSERT INTO query with all columns
				Input: inputs,
			}); err != nil {
				if attempt >= retryCount {
					return fmt.Errorf("clickhouse accumulator inserter: executing query on %q after %d retries: %w", acc.debugTableAndColumns(), attempt, err)
				}
				// Log, sleep, and get a fresh client before retrying
				i.logger.Warn("clickhouse insert failed, will retry", zap.Int("attempt", attempt+1), zap.Int("max_attempts", retryCount), zap.String("table", acc.tableName), zap.Error(err))
				time.Sleep(retrySleep)
				fresh, cErr := database.freshClient()
				if cErr != nil {
					return fmt.Errorf("clickhouse accumulator inserter: getting fresh client: %w", cErr)
				}
				client = fresh
				continue
			}
			break
		}

		queryDuration += time.Since(qStart)
	}

	//reset
	accs, err := createAccumulators(database.dialect)
	if err != nil {
		return fmt.Errorf("clickhouse accumulator inserter: creating accumulators: %w", err)
	}
	i.accumulators = accs

	i.logger.Debug("flushing done", zapx.HumanDuration("duration", time.Since(start)), zap.Int("rows", rowCount))

	return nil
}

func (acc *accumulator) debugTableAndColumns() string {
	var b strings.Builder
	b.WriteString(acc.tableName)
	b.WriteString(" (")
	for idx, col := range acc.columns {
		if idx > 0 {
			b.WriteString(", ")
		}
		b.WriteString(col.Name)
	}
	b.WriteString(")")
	return b.String()
}

type ColScaledDecimal256 struct {
	*proto.ColDecimal256
	scale uint8 // Your desired scale (0-9 for Decimal32)
}

func (c *ColScaledDecimal256) Type() proto.ColumnType {
	return proto.ColumnType(fmt.Sprintf("Decimal256(%d)", c.scale))
}

type ColScaledDecimal128 struct {
	*proto.ColDecimal128
	scale uint8 // Your desired scale (0-9 for Decimal32)
}

func (c *ColScaledDecimal128) Type() proto.ColumnType {
	return proto.ColumnType(fmt.Sprintf("Decimal128(%d)", c.scale))
}
