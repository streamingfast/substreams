package sql

import (
	"fmt"
	"strings"
)

// AllTables is what a per-table constraint switch takes to mean every table at once.
const AllTables = "all"

// ConstraintPolicy says which constraints the schema gets and when.
//
// Constraints are created after the load rather than during it — measured on 500k rows
// through binary COPY, loading with foreign keys in place costs 27.7x, while building the
// same constraints after the load costs 3.3x, for an identical schema. See
// TestConstraintCost in sink/sql/db_proto/benchmarks.

// ConstraintTiming says when the constraints are created.
type ConstraintTiming string

const (
	// ConstraintsAuto has the sink create them itself at the first live block, and only
	// there: a stop block ends a run without saying the backfill is over, and a range is
	// routinely one chunk of several. It is the default, stop-the-world pass and all. A backfill that ends with no primary keys and no foreign keys has
	// produced a database nobody should query, and leaving it that way until the operator
	// remembers a second command is the worse failure — it is silent, and it looks like
	// success.
	ConstraintsAuto ConstraintTiming = "auto"

	// ConstraintsManual leaves it to the operator, through `sink postgres constraints
	// apply`. Building them locks every table while indexes are built and every foreign
	// key is validated, so on a large database this is how the pass goes into a
	// maintenance window instead.
	ConstraintsManual ConstraintTiming = "manual"

	// ConstraintsAlways creates them before the first row is written, so the database
	// rejects bad data from the start and the load pays for it throughout.
	ConstraintsAlways ConstraintTiming = "always"
)

type ConstraintPolicy struct {
	// Timing decides when they are created; the zero value is ConstraintsAuto.
	Timing ConstraintTiming

	// DisableForeignKeys leaves out every foreign key, including the one to the block
	// table. Those are what a load pays most for.
	DisableForeignKeys bool

	// DisablePrimaryKeys and DisableUniques name the tables that go without, or AllTables.
	DisablePrimaryKeys []string
	DisableUniques     []string

	// DisableBlockNumberIndex leaves out the index on _block_number_. Every table carries
	// that column and the reorg path deletes by it on every table, so without the index
	// each undo is a sequential scan per table. It is only dead weight on a run that can
	// never reorg.
	//
	// Unlike the rest of this policy it is not about what the schema declares, and not
	// governed by Timing: the index is created when the sink starts.
	DisableBlockNumberIndex bool

	// PerTransaction is how many constraints are created or dropped per transaction.
	// Zero means one, which is what keeps the pass restartable and its memory bounded:
	// each index build and each foreign key validation is committed before the next
	// starts, so a run that is killed keeps what it finished and the next one carries on.
	// It is an execution knob rather than a property of the schema.
	PerTransaction int
}

// ConstraintsPerTransaction is how many statements the pass commits at a time.
func (p ConstraintPolicy) ConstraintsPerTransaction() int {
	if p.PerTransaction <= 0 {
		return 1
	}

	return p.PerTransaction
}

// SkipPrimaryKey reports whether the given table is meant to go without its primary key.
func (p ConstraintPolicy) SkipPrimaryKey(table string) bool {
	return matchesTable(p.DisablePrimaryKeys, table)
}

// SkipUnique reports whether the given table's unique constraints are meant to be left out.
func (p ConstraintPolicy) SkipUnique(table string) bool {
	return matchesTable(p.DisableUniques, table)
}

// SkipForeignKey reports whether the given table's foreign keys are meant to be left out.
func (p ConstraintPolicy) SkipForeignKey(string) bool {
	return p.DisableForeignKeys
}

// SkipsEverything reports a policy that declares no constraints, in which case there is
// nothing to apply and nothing to wait for. The block number index is not one of them: it
// is created when the sink starts, whatever the constraints say.
func (p ConstraintPolicy) SkipsEverything() bool {
	return p.DisableForeignKeys && matchesTable(p.DisablePrimaryKeys, AllTables) && matchesTable(p.DisableUniques, AllTables)
}

// DisableAllConstraints returns the policy that declares none of them, which is what an
// output with no schema annotations leaves the sink with. The block number index survives
// it: nothing in the annotations asks for that one, the reorg path does.
func DisableAllConstraints() ConstraintPolicy {
	return ConstraintPolicy{
		DisableForeignKeys: true,
		DisablePrimaryKeys: []string{AllTables},
		DisableUniques:     []string{AllTables},
	}
}

// WithBlockNumberIndex carries the index switch over from another policy, the index being
// the one thing that survives an output having no annotations to declare anything.
func (p ConstraintPolicy) WithBlockNumberIndex(from ConstraintPolicy) ConstraintPolicy {
	p.Timing = from.Timing
	p.PerTransaction = from.PerTransaction
	p.DisableBlockNumberIndex = from.DisableBlockNumberIndex

	return p
}

// Describe renders the policy for a log line.
func (p ConstraintPolicy) Describe() string {
	if p.SkipsEverything() {
		return "none"
	}

	var parts []string
	if p.DisableBlockNumberIndex {
		parts = append(parts, "no block number index")
	}
	if p.DisableForeignKeys {
		parts = append(parts, "no foreign keys")
	}
	if len(p.DisablePrimaryKeys) > 0 {
		parts = append(parts, fmt.Sprintf("no primary key on %s", strings.Join(p.DisablePrimaryKeys, ",")))
	}
	if len(p.DisableUniques) > 0 {
		parts = append(parts, fmt.Sprintf("no unique constraint on %s", strings.Join(p.DisableUniques, ",")))
	}
	if len(parts) == 0 {
		parts = append(parts, "all")
	}

	when := "created once the backfill is done"
	switch p.Timing {
	case ConstraintsAlways:
		when = "created before the load"
	case ConstraintsManual:
		when = "created by `sink postgres constraints apply`"
	}

	return strings.Join(parts, ", ") + ", " + when
}

func matchesTable(list []string, table string) bool {
	for _, entry := range list {
		entry = strings.TrimSpace(entry)
		if strings.EqualFold(entry, AllTables) || strings.EqualFold(entry, table) {
			return true
		}
	}

	return false
}

// ApplyUpfront reports whether the constraints go in before the first row.
func (p ConstraintPolicy) ApplyUpfront() bool {
	return p.Timing == ConstraintsAlways && !p.SkipsEverything()
}

// ApplyAtHead reports whether the sink creates them itself once the backfill is over.
func (p ConstraintPolicy) ApplyAtHead() bool {
	return (p.Timing == ConstraintsAuto || p.Timing == "") && !p.SkipsEverything()
}

// ParseConstraintTiming validates the flag value.
func ParseConstraintTiming(in string) (ConstraintTiming, error) {
	switch ConstraintTiming(in) {
	case "", ConstraintsAuto:
		return ConstraintsAuto, nil
	case ConstraintsManual:
		return ConstraintsManual, nil
	case ConstraintsAlways:
		return ConstraintsAlways, nil
	}

	return "", fmt.Errorf("invalid constraint timing %q, expected one of %q, %q or %q", in, ConstraintsAuto, ConstraintsManual, ConstraintsAlways)
}
