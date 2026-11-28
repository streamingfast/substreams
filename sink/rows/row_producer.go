package rows

import (
	"context"
	"errors"
	"iter"

	"google.golang.org/protobuf/reflect/protoreflect"
)

// Stream is the rows of one block, in emission order, as a one-pass iterator.
//
// Lifetime, retry and errors:
//   - A *Row is valid only until the next iteration step (the producer reuses it).
//   - A Stream is single-use: it cannot be rewound or resumed. To retry, after a failed delivery or to restart a
//     block after a crash, call [RowProducer.Rows] again with the same message. The rows come out in the same order
//     with the same Index and SeqBlock, because the walk is a pure, deterministic function of the message and the
//     schema. That is what lets a consumer resume in the middle of a block by skipping the rows up to the
//     SeqBlock it already delivered.
//   - An error is yielded as the last element (with a nil row) and ends the sequence. Rows already yielded stay
//     the consumer's to roll back. The producer keeps no state that survives the call and never mutates the
//     message, so retrying a block after an error is always safe. Breaking out of the loop early is safe too.
//   - The producer reports only its own errors (invalid message, failed check). They are deterministic for a given
//     message, retrying them is pointless, treat them as fatal for the block.
type Stream = iter.Seq2[*Row, error]

// RowProducer turns the module output of a block into rows.
type RowProducer interface {
	// Schema returns the schema rows refer to. It is final for static producers, a [Discoverable] one can grow it between blocks.
	Schema() *Schema
	// Rows yields the rows of one block. blk is the block context, available to the consumer as [Row.Block].
	Rows(blk BlockRef, msg protoreflect.Message) Stream
}

// SchemaProvider supplies a typed schema from outside the producer: configuration, database introspection, a file, or a
// previously persisted accumulated schema (see [SchemaStore]).
type SchemaProvider interface {
	ResolveSchema(ctx context.Context) (*Schema, error)
}

// Discoverable is implemented by producers that learn tables and columns at runtime, such as one reading
// DatabaseChanges. Growth is append-only: tables and columns are only added, so [TableID], [ColumnID] and the layout of
// [Row.Values] stay valid for rows already seen.
type Discoverable interface {
	RowProducer
	// SchemaVersion increments every time a table or a column is added.
	SchemaVersion() uint64
}

// ErrDiscoveredSchema is returned by [RequireStatic].
var ErrDiscoveredSchema = errors.New("the schema of this producer is discovered at runtime")

// RequireStatic fails when p discovers its schema at runtime, for the consumers that need it up front (parquet, SQL DDL).
func RequireStatic(p RowProducer) error {
	if _, ok := p.(Discoverable); ok {
		return ErrDiscoveredSchema
	}
	return nil
}
