package rows

import (
	"fmt"

	"google.golang.org/protobuf/reflect/protoreflect"
)

// BlockRowProducer yields exactly one row per block for a schema-less message: the table is named after the output
// module and its single column, `value`, holds the whole message as a [KindStruct]. It is what a plain, unannotated
// module output looks like as rows, and the degenerate case of the `blocks` delivery mode.
type BlockRowProducer struct {
	schema *Schema
}

// BlockValueColumn is the name of the single column of the [BlockRowProducer] table.
const BlockValueColumn = "value"

// NewBlockRowProducer builds the producer. The table is named by [WithModuleName], or after the message when it is not set.
func NewBlockRowProducer(root protoreflect.MessageDescriptor, opts ...Option) (*BlockRowProducer, error) {
	if root == nil {
		return nil, fmt.Errorf("nil root message descriptor")
	}
	o := newOptions(opts)

	name := o.moduleName
	if name == "" {
		name = string(root.Name())
	}

	s := &Schema{
		Root: root,
		Tables: []Table{{
			ID:      0,
			Name:    name,
			Message: root,
			Columns: []Column{{
				ID:   0,
				Name: BlockValueColumn,
				Type: LogicalType{Kind: KindStruct, Struct: root},
			}},
		}},
		maxColumns: 1,
	}
	s.index()

	return &BlockRowProducer{schema: s}, nil
}

// Schema implements [RowProducer].
func (p *BlockRowProducer) Schema() *Schema { return p.schema }

// Rows implements [RowProducer].
func (p *BlockRowProducer) Rows(blk BlockRef, msg protoreflect.Message) Stream {
	return func(yield func(*Row, error) bool) {
		if msg == nil || !msg.IsValid() {
			yield(nil, fmt.Errorf("block %d: nil or invalid message", blk.Number))
			return
		}

		row := &Row{Table: &p.schema.Tables[0], Op: OpCreate, Block: &blk, Values: []protoreflect.Value{protoreflect.ValueOfMessage(msg)}}
		row.Present.set(0)
		yield(row, nil)
	}
}
