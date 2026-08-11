package sql

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	pbSchema "github.com/streamingfast/substreams/pb/sf/substreams/sink/sql/schema/v1"
	sink "github.com/streamingfast/substreams/sink"
	"github.com/streamingfast/substreams/sink/sql/proto"
	"go.uber.org/zap"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type Database interface {
	FetchSinkInfo(schemaName string) (*SinkInfo, error)
	UpdateSinkInfoHash(schemaName string, newHash string) error
	StoreSinkInfo(schemaName string, schemaHash string) error

	CreateDatabase(useConstraints bool) error
	WalkMessageDescriptorAndInsert(dm *dynamicpb.Message, blockNum uint64, blockTimestamp time.Time, parent *Parent) (time.Duration, error)
	InsertBlock(blockNum uint64, hash string, timestamp time.Time) error

	HandleBlocksUndo(lastValidBlockNumber uint64) error

	FetchCursor() (*sink.Cursor, error)
	StoreCursor(cursor *sink.Cursor) error

	BeginTransaction() error
	CommitTransaction() error
	RollbackTransaction()
	Flush() (time.Duration, error)

	DatabaseHash(schemaName string) (uint64, error)

	GetDialect() Dialect

	Clone() Database
	Open() error
}

type BaseDatabase struct {
	logger                *zap.Logger
	mapOutputType         string
	insertStatements      map[string]*sql.Stmt
	RootMessageDescriptor protoreflect.MessageDescriptor
	useProtoOptions       bool
}

func NewBaseDatabase(moduleOutputType string, rootMessageDescriptor protoreflect.MessageDescriptor, useProtoOptions bool, logger *zap.Logger) (database *BaseDatabase, err error) {
	logger = logger.Named("database")

	return &BaseDatabase{
		logger:                logger,
		mapOutputType:         moduleOutputType,
		RootMessageDescriptor: rootMessageDescriptor,
		insertStatements:      make(map[string]*sql.Stmt),
		useProtoOptions:       useProtoOptions,
	}, nil
}

func (d *BaseDatabase) BaseClone() *BaseDatabase {
	return &BaseDatabase{
		logger:                d.logger,
		mapOutputType:         d.mapOutputType,
		RootMessageDescriptor: d.RootMessageDescriptor,
		insertStatements:      d.insertStatements,
	}
}

type Parent struct {
	field string
	id    interface{}
}

func (d *BaseDatabase) WalkMessageDescriptorAndInsertWithDialect(dm *dynamicpb.Message, blockNum uint64, blockTimestamp time.Time, parent *Parent, dialect Dialect, inserter Inserter) (time.Duration, error) {
	if dm == nil {
		return 0, fmt.Errorf("received a nil message")
	}

	var fieldValues []any
	fieldValues = append(fieldValues, blockNum)
	fieldValues = append(fieldValues, blockTimestamp)

	primaryKeyOffset := 2
	if dialect.UseVersionField() {
		fieldValues = append(fieldValues, time.Now().UnixNano())
		primaryKeyOffset += 1
	}

	if dialect.UseDeletedField() {
		fieldValues = append(fieldValues, false)
		primaryKeyOffset += 1
	}

	md := dm.Descriptor()
	tableInfo := proto.TableInfo(md)

	if tableInfo == nil && !d.useProtoOptions {
		tableInfo = &pbSchema.Table{
			Name: string(md.Name()),
		}
	}

	d.logger.Debug("Walking message descriptor", zap.String("message_descriptor_name", string(md.Name())), zap.Any("table_info", tableInfo))
	primaryKey := ""
	if tableInfo != nil {
		if table := dialect.GetTable(tableInfo.Name); table != nil {
			if table.PrimaryKey != nil {
				primaryKey = table.PrimaryKey.Name
				pkField := md.Fields().ByName(protoreflect.Name(primaryKey))
				if pkField == nil {
					return 0, fmt.Errorf("missing primary key field %q for table %q", primaryKey, tableInfo.Name)
				}
				pkValue := dm.Get(pkField)
				fieldValues = append(fieldValues, ScalarFieldValue(pkField, pkValue))
			}
		}
	}

	totalSqlDuration := time.Duration(0)

	if parent != nil {
		fieldValues = append(fieldValues, parent.id)
	}

	var childs []*dynamicpb.Message

	fields := md.Fields()
	for i := 0; i < fields.Len(); i++ {
		fd := fields.Get(i)
		if string(fd.Name()) == primaryKey {
			continue
		}
		fv := dm.Get(fd)

		if fd.IsList() {
			// Check if this is an array of messages or native values
			list := fv.List()
			if fd.Kind() == protoreflect.MessageKind {
				// Check if this is an inline nested array
				fieldInfo := proto.FieldInfo(fd)
				if fieldInfo != nil && fieldInfo.Inline {
					// Delegate to dialect for inline handling
					var err error
					fieldValues, err = dialect.AppendInlineFieldValues(fieldValues, fd, fv, dm)
					if err != nil {
						return 0, fmt.Errorf("appending inline field values for %q: %w", string(fd.Name()), err)
					}
				} else if list.Len() > 0 {
					// Array of messages - process as child tables
					for j := 0; j < list.Len(); j++ {
						fm := list.Get(j).Message().Interface().(*dynamicpb.Message)
						childs = append(childs, fm)
					}
				}
			} else if list.Len() > 0 {
				// Array of native values - add as a single field value (the array itself)
				var values []interface{}
				for j := 0; j < list.Len(); j++ {
					values = append(values, ScalarFieldValue(fd,list.Get(j)))
				}
				fieldValues = append(fieldValues, values)
			} else {
				fieldValues = append(fieldValues, []interface{}{})
			}
		} else if fd.Kind() == protoreflect.MessageKind {
			if fv.Message().IsValid() {
				fm := fv.Message().Interface().(*dynamicpb.Message)
				if fm.Descriptor().FullName() == "google.protobuf.Timestamp" {
					// Convert fv to *timestamppb.Timestamp
					timestamp := &timestamppb.Timestamp{}
					timestamp.Seconds = fm.Get(fm.Descriptor().Fields().ByName("seconds")).Int()
					timestamp.Nanos = int32(fm.Get(fm.Descriptor().Fields().ByName("nanos")).Int())
					fieldValues = append(fieldValues, timestamp)
					continue
				}

				// Check if this field should be treated as a nested (inline) column
				fieldInfo := proto.FieldInfo(fd)
				if fieldInfo != nil && fieldInfo.Inline {
					// Delegate to dialect for inline handling
					var err error
					fieldValues, err = dialect.AppendInlineFieldValues(fieldValues, fd, fv, dm)
					if err != nil {
						return 0, fmt.Errorf("appending inline field values for %q: %w", string(fd.Name()), err)
					}
					continue
				}

				childs = append(childs, fm) //need to be handled after current message inserted
			}
		} else {
			fieldValues = append(fieldValues, ScalarFieldValue(fd,fv))
		}
	}

	var p *Parent

	if tableInfo != nil {
		insertStartAt := time.Now()
		table := dialect.GetTable(tableInfo.Name)
		if table != nil {
			err := inserter.Insert(table.Name, fieldValues)
			if err != nil {
				d.logger.Info("failed to insert into table, printing field values for debugging", zap.String("table_name", table.Name), zap.Any("field_values", fieldValues))
				return 0, fmt.Errorf("inserting into table %q: %w", table.Name, err)
			}
			if len(childs) > 0 && d.useProtoOptions {
				if table.PrimaryKey == nil {
					for _, child := range childs {
						fmt.Println("child:", child.Descriptor().FullName())
					}
					return 0, fmt.Errorf("table %q has no primary key and has %d associated children table", table.Name, len(childs))
				}
				idx := table.PrimaryKey.Index + primaryKeyOffset
				id := fieldValues[idx]
				p = &Parent{
					field: strings.ToLower(string(md.Name())),
					id:    id,
				}
			}
			totalSqlDuration += time.Since(insertStartAt)
		}
	}

	for _, fm := range childs {
		sqlDuration, err := d.WalkMessageDescriptorAndInsertWithDialect(fm, blockNum, blockTimestamp, p, dialect, inserter)
		if err != nil {
			return 0, fmt.Errorf("processing child %q: %w", string(fm.Descriptor().FullName()), err)
		}
		totalSqlDuration += sqlDuration
	}

	return totalSqlDuration, nil
}

// ScalarFieldValue unwraps a non-message, non-list value for the inserters.
//
// Enums are the only kind that cannot be handed over as-is: protoreflect yields a
// protoreflect.EnumNumber, a named int32 type that every dialect's type switch misses,
// so both of them panicked on any message carrying an enum field.
//
// Every path that feeds a value to an inserter has to go through here, including the
// list elements and the dialects' inline/nested extraction, or the same panic comes
// back for a repeated or nested enum.
func ScalarFieldValue(fd protoreflect.FieldDescriptor, value protoreflect.Value) any {
	if fd.Kind() != protoreflect.EnumKind {
		return value.Interface()
	}

	number := value.Enum()
	out := EnumValue{Number: int32(number)}
	if descriptor := fd.Enum().Values().ByNumber(number); descriptor != nil {
		out.Name = string(descriptor.Name())
	}

	return out
}

type SinkInfo struct {
	SchemaHash string `json:"schema_hash"`
}
