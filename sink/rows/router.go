package rows

import (
	"errors"
	"fmt"
	"strings"

	"google.golang.org/protobuf/reflect/protoreflect"

	"github.com/streamingfast/protox"
	schemapb "github.com/streamingfast/substreams/pb/sf/substreams/sink/v1"
)

//go:generate go-enum -f=$GOFILE --marshal --names

// RowProducerKind selects a producer. [RowProducerKindAuto] picks from the output module.
//
// ENUM(auto, schema, database_changes, block)
type RowProducerKind uint8

// ErrRowProducerUnavailable is returned when the selected producer is designed but not implemented yet.
var ErrRowProducerUnavailable = errors.New("producer is not available")

// databaseChangesTypes are the output types, without the `proto:` prefix, that carry table changes.
var databaseChangesTypes = []string{
	"sf.substreams.sink.database.v1.DatabaseChanges",
	"sf.substreams.database.v1.DatabaseChanges",
}

// DetectRowProducerKind resolves [RowProducerKindAuto] from the output module. outputType can carry the `proto:` prefix, desc
// can be nil when the descriptor is not known. DatabaseChanges gives the database changes producer, a message with
// explicit table annotations gives the schema producer, anything else the block producer.
func DetectRowProducerKind(outputType string, desc protoreflect.MessageDescriptor) RowProducerKind {
	unprefixed := strings.TrimPrefix(outputType, "proto:")
	for _, candidate := range databaseChangesTypes {
		if unprefixed == candidate {
			return RowProducerKindDatabaseChanges
		}
	}

	if HasTableSchema(desc) {
		return RowProducerKindSchema
	}
	return RowProducerKindBlock
}

// NewRowProducer builds the producer for an output module. An explicit kind is used as is, [RowProducerKindAuto] goes
// through [DetectRowProducerKind]. The database changes producer is designed for but not implemented yet and fails with
// [ErrRowProducerUnavailable].
func NewRowProducer(kind RowProducerKind, outputType string, desc protoreflect.MessageDescriptor, opts ...Option) (RowProducer, error) {
	if kind == RowProducerKindAuto {
		kind = DetectRowProducerKind(outputType, desc)
	}

	switch kind {
	case RowProducerKindSchema:
		if desc == nil {
			return nil, errors.New("the schema producer needs the output message descriptor")
		}
		return NewSchemaRowProducer(desc, opts...)
	case RowProducerKindBlock:
		if desc == nil {
			return nil, errors.New("the block producer needs the output message descriptor")
		}
		return NewBlockRowProducer(desc, opts...)
	case RowProducerKindDatabaseChanges:
		return nil, fmt.Errorf("%w: %s", ErrRowProducerUnavailable, kind)
	default:
		return nil, fmt.Errorf("unknown producer kind %d", kind)
	}
}

// HasTableSchema tells whether desc, or a message reachable through the fields [Resolve] follows (see [isChildField]), carries a
// `(schema.table)` option. It follows exactly the fields Resolve does, so detecting a schema and resolving it always agree.
func HasTableSchema(desc protoreflect.MessageDescriptor) bool {
	return hasTableSchema(desc, map[protoreflect.FullName]bool{})
}

func hasTableSchema(desc protoreflect.MessageDescriptor, visited map[protoreflect.FullName]bool) bool {
	if desc == nil || visited[desc.FullName()] {
		return false
	}
	visited[desc.FullName()] = true

	if table, found := protox.GetMessageExtensionValue[*schemapb.Table](desc, schemapb.E_Table, nil); found && table != nil {
		return true
	}

	fields := desc.Fields()
	for i := range fields.Len() {
		if fd := fields.Get(i); isChildField(fd) && hasTableSchema(fd.Message(), visited) {
			return true
		}
	}
	return false
}
