package sql

import "fmt"

// BufferedInserter records the inserts a message walk produces instead of performing
// them, so the walk can run off the goroutine that owns the database.
//
// It is not safe for concurrent use: each worker owns one. Replay puts the recorded
// inserts back in the exact order the walk produced them, which is what keeps a
// parent row ahead of its children.
type BufferedInserter struct {
	ops []bufferedOp
}

type bufferedOp struct {
	table  string
	values []any
}

// NewBufferedInserter returns a buffer sized for roughly the given number of rows.
func NewBufferedInserter(expectedRows int) *BufferedInserter {
	return &BufferedInserter{ops: make([]bufferedOp, 0, expectedRows)}
}

func (b *BufferedInserter) Insert(table string, values []any) error {
	b.ops = append(b.ops, bufferedOp{table: table, values: values})

	return nil
}

// Rows returns how many inserts are buffered.
func (b *BufferedInserter) Rows() int { return len(b.ops) }

// Replay performs the buffered inserts against target, in order.
func (b *BufferedInserter) Replay(target Inserter) error {
	for _, op := range b.ops {
		if err := target.Insert(op.table, op.values); err != nil {
			return fmt.Errorf("inserting buffered row into %q: %w", op.table, err)
		}
	}

	return nil
}

// Reset drops the buffered inserts, keeping the backing array for reuse.
func (b *BufferedInserter) Reset() {
	clear(b.ops)
	b.ops = b.ops[:0]
}
