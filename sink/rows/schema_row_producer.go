package rows

import (
	"fmt"
	"sync"

	"google.golang.org/protobuf/reflect/protoreflect"
)

// SchemaRowProducer yields the rows of a message tree annotated with `(schema.table)` and `(schema.field)`
// options (or of any message tree when annotations are not required). Rows come out in pre-order: a row,
// then its children, fields in descriptor order and list elements in order, which is the order db_proto
// inserts them in.
type SchemaRowProducer struct {
	schema *Schema
	check  bool
	pool   sync.Pool
}

// NewSchemaRowProducer resolves the schema of root and returns the producer for it.
func NewSchemaRowProducer(root protoreflect.MessageDescriptor, opts ...Option) (*SchemaRowProducer, error) {
	o := newOptions(opts)

	schema, err := Resolve(root, opts...)
	if err != nil {
		return nil, err
	}

	p := &SchemaRowProducer{schema: schema, check: o.check}
	p.pool.New = func() any { return p.newWalker() }
	return p, nil
}

// Schema implements [RowProducer].
func (p *SchemaRowProducer) Schema() *Schema { return p.schema }

// Rows implements [RowProducer]. It is safe to call concurrently: every call gets its own walker.
func (p *SchemaRowProducer) Rows(blk BlockRef, msg protoreflect.Message) Stream {
	return func(yield func(*Row, error) bool) {
		if msg == nil || !msg.IsValid() {
			yield(nil, fmt.Errorf("block %d: nil or invalid message", blk.Number))
			return
		}

		w := p.pool.Get().(*walker)
		defer func() {
			w.release()
			p.pool.Put(w)
		}()
		w.reset(blk)

		// The walk reads fields through descriptors cached at resolve time, and protobuf-go panics when a field
		// descriptor from another registry is used on a message. Every supported message implementation (generated,
		// dynamicpb, hyperpb) reports the descriptor it was built from, so a mismatch is caught here, once per block.
		if md := msg.Descriptor(); md != p.schema.Root {
			yield(nil, fmt.Errorf("block %d: message %q was not built from the descriptor instance the schema was resolved from, build it from the same descriptor (a copy of %q from another registry is not accepted)", blk.Number, md.FullName(), p.schema.Root.FullName()))
			return
		}

		w.walk(msg, p.schema.rootPlan, nil, 0, 0, yield)
	}
}

type childRef struct {
	msg  protoreflect.Message
	plan *msgPlan
}

// walker holds the per-call state: frames reused by nesting depth, per-table counters and child scratch space.
type walker struct {
	p        *SchemaRowProducer
	blk      BlockRef
	seq      uint32
	counters []uint32
	frames   []*Row
	scratch  [][]childRef
}

func (p *SchemaRowProducer) newWalker() *walker {
	return &walker{p: p, counters: make([]uint32, len(p.schema.Tables))}
}

func (w *walker) reset(blk BlockRef) {
	w.blk = blk
	w.seq = 0
	clear(w.counters)
}

func (w *walker) frame(depth int) *Row {
	for len(w.frames) <= depth {
		w.frames = append(w.frames, &Row{Values: make([]protoreflect.Value, w.p.schema.maxColumns)})
	}
	return w.frames[depth]
}

// release drops what the pooled frames still reference, so an idle walker does not keep a decoded block (and a hyperpb
// arena) alive after Rows returned.
func (w *walker) release() {
	for _, frame := range w.frames {
		clear(frame.Values[:cap(frame.Values)])
		frame.Parent, frame.Table, frame.Block = nil, nil, nil
	}
	for i := range w.scratch {
		clear(w.scratch[i][:cap(w.scratch[i])])
	}
}

// walk emits msg (when it maps to a table) and then its children. depth is the number of emitted ancestors,
// level the number of messages walked, they differ through pass-through messages. It returns false when the
// consumer stopped or an error was yielded.
func (w *walker) walk(msg protoreflect.Message, plan *msgPlan, parent *Row, depth, level int, yield func(*Row, error) bool) bool {
	var row *Row
	if plan.hasTbl {
		table := plan.table

		row = w.frame(depth)
		row.Table, row.Op, row.Block, row.Parent = table, OpCreate, &w.blk, parent
		row.Index = w.counters[table.ID]
		w.counters[table.ID]++
		row.SeqBlock = w.seq
		w.seq++
		row.Values = row.Values[:len(table.Columns)]
		row.Present.reset()
	}

	for len(w.scratch) <= level {
		w.scratch = append(w.scratch, nil)
	}
	children := w.scratch[level][:0]

	for i := range plan.fields {
		fp := &plan.fields[i]

		switch fp.kind {
		case planColumn:
			if row == nil {
				continue
			}

			if fp.presence && !msg.Has(fp.field) {
				row.Values[fp.col] = protoreflect.Value{}
				continue
			}

			value := msg.Get(fp.field)
			if w.p.check && fp.check != checkNone {
				if err := w.checkValue(row, fp, value); err != nil {
					yield(nil, err)
					return false
				}
			}
			row.Values[fp.col] = value
			row.Present.set(fp.col)

		case planChild:
			if msg.Has(fp.field) {
				children = append(children, childRef{msg: msg.Get(fp.field).Message(), plan: fp.child})
			}

		case planChildList:
			list := msg.Get(fp.field).List()
			for j := range list.Len() {
				children = append(children, childRef{msg: list.Get(j).Message(), plan: fp.child})
			}
		}
	}
	w.scratch[level] = children

	if row != nil && !yield(row, nil) {
		return false
	}

	next := parent
	nextDepth := depth
	if row != nil {
		next, nextDepth = row, depth+1
	}

	for _, child := range children {
		if !w.walk(child.msg, child.plan, next, nextDepth, level+1, yield) {
			return false
		}
	}
	return true
}

func (w *walker) checkValue(row *Row, fp *fieldPlan, value protoreflect.Value) error {
	column := &row.Table.Columns[fp.col]

	if column.Type.Kind == KindList {
		list := value.List()
		for i := range list.Len() {
			if err := checkScalar(fp.check, column, list.Get(i).String()); err != nil {
				return w.checkError(row, column, list.Get(i).String(), err)
			}
		}
		return nil
	}

	if err := checkScalar(fp.check, column, value.String()); err != nil {
		return w.checkError(row, column, value.String(), err)
	}
	return nil
}

func (w *walker) checkError(row *Row, column *Column, value string, err error) error {
	return &ValidationError{Block: w.blk, Table: row.Table.Name, Column: column.Name, Value: truncate(value), Reason: err.Error()}
}
